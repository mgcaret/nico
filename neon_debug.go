package main

import (
	"fmt"
	"github.com/jacobsa/go-serial/serial"
	"github.com/marcinbor85/gohex"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	debugSpeed uint = 57600 // console serial speed
)

// *** Debug Interfaces ***

// These connect to the debug interface of the target system
// A servicer is run as a goroutine
// They take commands on the debug command channel in the form of
// words []string and send their results via the debug output channel

// This one does nothing but eat the debug input stream and report
// an error, in the case where the debug and command inputs are
// in use, but the debug interface is not connected
func nullDebugInterface() {
	for {
		select {
		case words := <-debugCommandChan:
			debugOutputChan <- fmt.Sprintf("Bad command: %s\n", words[0])
		}
	}
}

// To be used when the -no-debug option is specified, just eat any
// commands that come from the channel
func noDebugInterface() {
	for {
		select {
		case <-debugCommandChan:
			// nothing
		}
	}
}

// Return a debugger interface for the given device.  In this case, we need to
// abstract the socket or serial device so that we can share the command
// processing structure, and make sure that the debug interface has access to
// them.  Note the debug interface is synchronous.
func getDebugInterface(device string) debugInterfaceFunc {
	var deviceReadWriter io.ReadWriter
	fi, err := os.Stat(device)
	if err != nil {
		debugOutputChan <- "Error accessing debug interface device\n"
		return nil
	}
	if (fi.Mode() & os.ModeSocket) != 0 {
		debugSpeed = 0
		deviceReadWriter, err = net.Dial("unix", device)
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Failed to connect to socket %s: %v", device, err)
			return nil
		}
	} else if (fi.Mode() & os.ModeDevice) != 0 {
		options := serial.OpenOptions{
			PortName:        device,
			BaudRate:        debugSpeed,
			DataBits:        8,
			StopBits:        1,
			MinimumReadSize: 0,
			InterCharacterTimeout: 1000,
		}
		deviceReadWriter, err = serial.Open(options)
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Failed to connect to device %s: %v", device, err)
			return nil
		}
	}
	if deviceReadWriter == nil {
		debugOutputChan <- "Invalid debug interface device"
		return nil
	}
	debugOutputChan <- fmt.Sprintf("Connected to debugger at %s\n", device)
	return func() {
		debugResync(deviceReadWriter)
		for {
			select {
			case words := <-debugCommandChan:
				doDebugCommand(deviceReadWriter, words)
			}
		}
	}
}
func doDebugCommand(rw io.ReadWriter, words []string) {
CmdSwitch:
	switch strings.ToLower(words[0]) {
	case "stop":
		rw.Write([]byte("]"))
		debugOutputChan <- "Stop sent!\n"
	case "cont", "go":
		rw.Write([]byte("["))
		debugOutputChan <- "Go sent!\n"
	case "reset":
		rw.Write([]byte("R"))
		debugOutputChan <- "Reset sent!\n"
	case "step":
		rw.Write([]byte("X"))
		debugOutputChan <- "Step sent!\n"
	case "run":
		rw.Write([]byte("]R["))
		debugOutputChan <- "Run sent!\n"
	case "read":
		args := words[1:]
		for _, a := range args {
			if a == "" {
				continue // ignore empty strings
			}
			addr, err := strconv.ParseUint(a, 0, 24)
			if err != nil {
				debugOutputChan <- fmt.Sprintf("Bad address: %s\n", a)
				continue
			}
			debugWriteHex(rw, uint(addr>>16), 2)
			debugWriteChars(rw, ":")
			debugWriteHex(rw, uint(addr), 4)
			debugWriteChars(rw, "#")
			for j := 0; j < 4; j++ {
				buf := make([]uint, 16)
				for i := 0; i < 16; i++ {
					debugWriteChars(rw, "@")
					buf[i] = debugReadByte(rw)
				}
				debugOutputChan <- fmt.Sprintf("%08X  ", addr)
				for i, v := range buf {
					if i == 8 {
						debugOutputChan <- " "
					}
					debugOutputChan <- fmt.Sprintf("%02X ", v)
				}
				debugOutputChan <- "["
				for _, v := range buf {
					if v >= 32 && v < 127 {
						debugOutputChan <- string(v)
					} else {
						debugOutputChan <- " "
					}
				}
				debugOutputChan <- "]\n"
				addr += 16
			}
		}
	case "write":
		args := words[1:]
		haveAddr := false
		byteCount := 0
		for _, a := range args {
			if a == "" {
				continue // ignore empty strings
			}
			if haveAddr {
				dat, err := strconv.ParseUint(a, 0, 8)
				if err != nil {
					debugOutputChan <- fmt.Sprintf("Bad data: %s\n", a)
					break
				}
				debugWriteHex(rw, uint(dat), 2)
				debugWriteChars(rw, "!")
				byteCount++
			} else {
				addr, err := strconv.ParseUint(a, 0, 24)
				if err != nil {
					debugOutputChan <- fmt.Sprintf("Bad address: %s\n", a)
					break
				}
				debugWriteHex(rw, uint(addr>>16), 2)
				debugWriteChars(rw, ":")
				debugWriteHex(rw, uint(addr), 4)
				debugWriteChars(rw, "#")
				haveAddr = true
			}
		}
		plural := "s"
		if byteCount == 1 {
			plural = ""
		}
		debugOutputChan <- fmt.Sprintf("Wrote %v byte%s!\n", byteCount, plural)
	case "program", "verify":
		program := strings.ToLower(words[0]) == "program"
		args := words[1:]
		if len(args) > 0 && args[0] == "" {
			args = args[1:]
		}
		if len(args) == 0 {
			debugOutputChan <- "No file specified!\n"
			break // out of switch
		}
		file, err := os.Open(args[0])
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Could not open %s: %v\n", args[0], err)
			break
		}
		defer file.Close()
		mem := gohex.NewMemory()
		err = mem.ParseIntelHex(file)
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Could not parse %s: %v\n", args[0], err)
			break
		}
		debugWriteChars(rw, "]R")
		for segNum, segment := range mem.GetDataSegments() {
			plural := "s"
			if len(segment.Data) == 1 {
				plural = ""
			}
			if program {
				debugOutputChan <- "Programming"
			} else {
				debugOutputChan <- "Verifying"
			}
			debugOutputChan <- fmt.Sprintf(
				" segment %v at 0x%08x, %v byte%s\n",
				segNum, segment.Address, len(segment.Data),
				plural)
			debugWriteHex(rw, uint(segment.Address>>16), 2)
			debugWriteChars(rw, ":")
			debugWriteHex(rw, uint(segment.Address), 4)
			debugWriteChars(rw, "#")
			for idx, dat := range segment.Data {
				if idx != 0 && ((segment.Address+uint32(idx))&0xFFFF == 0) {
					// roll over to next bank
					debugWriteHex(rw, uint(segment.Address+uint32(idx))>>16, 2)
					debugWriteChars(rw, ":")
					debugWriteHex(rw, 0, 4)
					debugWriteChars(rw, "#")
				}
				if program {
					debugWriteHex(rw, uint(dat), 2)
					debugWriteChars(rw, "!")
				} else {
					debugWriteChars(rw, "@")
					b := debugReadByte(rw)
					if b != uint(dat) {
						debugOutputChan <- fmt.Sprintf("Validation failed at %08x!\n", segment.Address+uint32(idx))
						break CmdSwitch
					}
				}
			}
			debugOutputChan <- "Segment complete!\n"
		}
		if program {
			debugOutputChan <- "Program complete!\n"
		} else {
			debugOutputChan <- "Verify complete!\n"
		}
	case "flash", "verify-rom":
		program := strings.ToLower(words[0]) == "flash"
		args := words[1:]
		if len(args) > 0 && args[0] == "" {
			args = args[1:]
		}
		if len(args) == 0 {
			debugOutputChan <- "No file specified!\n"
			break // out of switch
		}
		file, err := os.Open(args[0])
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Could not open %s: %v\n", args[0], err)
			break
		}
		defer file.Close()
		mem := gohex.NewMemory()
		err = mem.ParseIntelHex(file)
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Could not parse %s: %v\n", args[0], err)
			break
		}
		debugWriteChars(rw, "]R")
		if program {
			debugOutputChan <- "Erasing chip...\n"
			debugEraseChip(rw, 0x20)
			debugOutputChan <- "Flashing...\n"
		} else {
			debugOutputChan <- "Verifying...\n"
		}
		for segNum, segment := range mem.GetDataSegments() {
			plural := "s"
			if len(segment.Data) == 1 {
				plural = ""
			}
			// Force segments into flash address space
			segAddr := 0x20_0000 | (segment.Address & 0x0F_FFFF)
			if program {
				debugOutputChan <- "Programming"
			} else {
				debugOutputChan <- "Verifying"
			}
			debugOutputChan <- fmt.Sprintf(
				" segment %v at 0x%08x, %v byte%s\n",
				segNum, segAddr, len(segment.Data),
				plural)
			debugWriteHex(rw, uint(segAddr>>16), 2)
			debugWriteChars(rw, ":")
			if !program {
				debugWriteHex(rw, 0, 4)
				debugWriteChars(rw, "#")
			}
			for idx, dat := range segment.Data {
				if idx&0x7FF == 0 {
					debugOutputChan <- fmt.Sprintf("%v%%\r", idx*100/len(segment.Data))
				}
				if idx != 0 && ((segAddr+uint32(idx))&0xFFFF == 0) {
					// roll over to next bank
					debugWriteHex(rw, uint(segAddr+uint32(idx))>>16, 2)
					debugWriteChars(rw, ":")
					if !program {
						debugWriteHex(rw, 0, 4)
						debugWriteChars(rw, "#")
					}
				}
				if program {
					// Since the erased chip is all 0xFF, we won't write those.
					// This will speed up flashing.
					if dat != 0xFF {
						debugWriteCmd(rw, 0x5555, 0xAA)
						debugWriteCmd(rw, 0x2AAA, 0x55)
						debugWriteCmd(rw, 0x5555, 0xA0)
						debugWriteCmd(rw, uint(segAddr+uint32(idx))&0xFFFF, uint(dat))
						//debugCmdWait(20) // probably not needed
					}
				} else {
					debugWriteChars(rw, "@")
					b := debugReadByte(rw)
					if b != uint(dat) {
						debugOutputChan <- fmt.Sprintf("Validation failed at %08x!\n", segAddr+uint32(idx))
						break CmdSwitch
					}
				}
			}
			debugOutputChan <- "Segment complete!\n"
		}
		if program {
			debugOutputChan <- "Flash"
		} else {
			debugOutputChan <- "Verify"
		}
		debugOutputChan <- " complete!\n"
	case "mapram":
		debugWriteChars(rw, "]R")
		debugWriteHex(rw, 0x08, 2)
		debugWriteChars(rw, ":")
		for i := uint(0); i < 16; i++ {
			debugWriteCmd(rw, 2*i, i*16)
			debugWriteCmd(rw, 2*i+1, 0x80)
		}
		debugOutputChan <- "RAM mapped to bank 0!\n"
	case "erase":
		debugEraseChip(rw, 0x20)
		debugOutputChan <- "Flash ROM erased!\n"
	case "chipid":
		id0, id1 := debugChipID(rw, 0x20)
		debugOutputChan <- "Manufacturer: "
		switch id0 {
		case 0xBF:
			debugOutputChan <- "SST, device: "
			switch id1 {
			case 0xD5:
				debugOutputChan <- "39xF010 (128K)\n"
			case 0xD6:
				debugOutputChan <- "39xF020 (256K)\n"
			case 0xD7:
				debugOutputChan <- "39xF040 (512K)\n"
			default:
				debugOutputChan <- fmt.Sprintf("0x%02X\n", id1)
			}
		default:
			debugOutputChan <- fmt.Sprintf("0x%02X, device: 0x%02X\n", id0, id1)
		}
	case "resync":
		debugResync(rw)
	default:
		debugOutputChan <- fmt.Sprintf("Unknown command: '%s'\n", words[0])
	}
}

func debugWriteChars(rw io.ReadWriter, s string) {
	var err error
	obuf := make([]byte, 1)
	for _, b := range []byte(s) {
		obuf[0] = b
		_, err = rw.Write(obuf)
		if err != nil {
			break
		}
		if debugSpeed > 0 {
			// Pace characters so we don't overwhelm the receive buffer
			time.Sleep((10000000 / time.Duration(debugSpeed)) * time.Microsecond)
		}
	}
	if err != nil {
		debugOutputChan <- fmt.Sprintf("Error writing to debug device: %v\n", err)
	}
}

func debugWriteHex(rw io.ReadWriter, v uint, l uint) {
	for i := uint(0); i < l; i++ {
		n := (v >> (4 * (l - i - 1))) & 0xF
		debugWriteChars(rw, fmt.Sprintf("%X", n))
	}
}

func debugWriteCmd(rw io.ReadWriter, addr uint, dat uint) {
	debugWriteHex(rw, addr, 4)
	debugWriteChars(rw, "#")
	debugWriteHex(rw, dat, 2)
	debugWriteChars(rw, "!")
}

func debugCmdWait(t int) {
	// Can't sync with go-serial, unfortunately
	// just have to hope this works
	time.Sleep(time.Duration(t) * time.Microsecond)
}

func debugReadByte(rw io.ReadWriter) uint {
	buf := make([]byte, 2)
	_, err := rw.Read(buf)
	if err != nil {
		debugOutputChan <- fmt.Sprintf("Error reading from debug device: %v\n", err)
	} else {
		i, err := strconv.ParseUint(string(buf), 16, 8)
		if err != nil {
			debugOutputChan <- fmt.Sprintf("Error parsing data from debug device: %v\n", err)
		}
		return uint(i)
	}
	return 0
}

func debugEraseChip(rw io.ReadWriter, bank uint) {
	// Stop & reset
	debugWriteChars(rw, "]R")
	// Send bank of flash chip
	debugWriteHex(rw, bank, 2)
	debugWriteChars(rw, ":")
	// Erase chip
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteCmd(rw, 0x5555, 0x80)
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteCmd(rw, 0x5555, 0x10)
	debugCmdWait(100000)
}

func debugEraseSector(rw io.ReadWriter, sa uint) {
	// Stop
	debugWriteChars(rw, "]")
	// Send bank of flash chip
	debugWriteHex(rw, (sa >> 16) & 0xF0, 2)
	debugWriteChars(rw, ":")
	// Erase sector
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteCmd(rw, 0x5555, 0x80)
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteHex(rw, sa >> 16, 2)
	debugWriteChars(rw, ":")
	debugWriteCmd(rw, sa & 0xFFFF, 0x30)
	debugCmdWait(50000)
}

func debugChipID(rw io.ReadWriter, bank uint) (uint, uint) {
	// Stop
	debugWriteChars(rw, "]")
	// Send bank of flash chip
	debugWriteHex(rw, bank, 2)
	debugWriteChars(rw, ":")
	// Software ID mode enter
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteCmd(rw, 0x5555, 0x90)
	// Now read the ID bytes
	debugWriteHex(rw, 0, 4)
	debugWriteChars(rw, "#")
	debugWriteChars(rw, "@")
	id0 := debugReadByte(rw)
	debugWriteChars(rw, "@")
	id1 := debugReadByte(rw)
	// Software ID mode exit
	debugWriteCmd(rw, 0x5555, 0xAA)
	debugWriteCmd(rw, 0x2AAA, 0x55)
	debugWriteCmd(rw, 0x5555, 0xF0)
	return id0, id1
}

func debugResync(rw io.ReadWriter) int {
	buf := make([]byte, 16)
	n, _ := rw.Read(buf)
	if n > 0 {
		plural := "s"
		if n == 1 {
			plural = ""
		}
		debugOutputChan <- fmt.Sprintf("Discarded %v byte%s from debug device.\n", n, plural)
	}
	return n
}
