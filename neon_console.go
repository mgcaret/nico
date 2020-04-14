package main

import (
	"fmt"
	"github.com/gbin/goncurses"
	"github.com/jacobsa/go-serial/serial"
	"net"
	"os"
	"time"
)

var (
	consoleSpeed uint = 9600 // console serial speed
)

// *** Console I/O servicers ***

// These perform console I/O with the target system

// demo Console I/O servicer, just copies input to output and fills
// the debug window with junk
func demoConsoleIoServicer() {
	for {
		select {
		case k := <-consoleInputChan:
			consoleOutputChan <- goncurses.KeyString(k)
			debugOutputChan <- fmt.Sprintf("<%s,%v>", goncurses.KeyString(k), k)
		}
	}
}

// This returns an appropriate console I/O servicer depending on what device
// the user has specified.   We support sockets for both testing purposes
// and connecting to a possible future emulator's serial port.
func getConsoleIoServicer(device string) consoleIoServicerFunc {
	fi, err := os.Stat(device)
	if err != nil {
		quitChan <- "Error accessing console device"
		return nil
	}
	if (fi.Mode() & os.ModeSocket) != 0 {
		return getSocketConsoleIoServicer(device)
	} else if (fi.Mode() & os.ModeDevice) != 0 {
		return getSerialConsoleIoServicer(device)
	}
	quitChan <- "Invalid console device"
	return nil // n
}

// Return a socket console I/O servicer
func getSocketConsoleIoServicer(device string) consoleIoServicerFunc {
	socketReadChan := make(chan string)
	c, err := net.Dial("unix", device)
	if err != nil {
		quitChan <- fmt.Sprintf("Failed to connect to socket %s: %v", device, err)
		return nil
	}
	consoleOutputChan <- fmt.Sprintf("Connected to socket: %s\r\n", device)
	go func() {
		ibuf := make([]byte, 1024)
		for {
			n, err := c.Read(ibuf)
			if err != nil {
				quitChan <- fmt.Sprintf("Error reading from %s: %v", device, err)
			}
			socketReadChan <- string(ibuf[0:n])
		}
	}()
	return func() {
		obuf := make([]byte, 1)
		for {
			select {
			case k := <-consoleInputChan:
				obuf[0] = byte(k)
				_, err := c.Write(obuf)
				if err != nil {
					quitChan <- fmt.Sprintf("Error writing to %s: %v", device, err)
				}
			case s := <-socketReadChan:
				consoleOutputChan <- s
			}
		}
	}
}

// Return a serial console I/O servicer
func getSerialConsoleIoServicer(device string) consoleIoServicerFunc {
	options := serial.OpenOptions{
		PortName:        device,
		BaudRate:        consoleSpeed,
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: 1,
	}
	serialReadChan := make(chan string)
	serialWriteChan := make(chan byte, 256)
	c, err := serial.Open(options)
	if err != nil {
		quitChan <- fmt.Sprintf("Failed to connect to device %s: %v", device, err)
		return nil
	}
	consoleOutputChan <- fmt.Sprintf("Connected to device: %s\r\n", device)
	go func() {
		ibuf := make([]byte, 1)
		for {
			n, err := c.Read(ibuf)
			if err != nil {
				quitChan <- fmt.Sprintf("Error reading from %s: %v", device, err)
				c.Close()
			}
			serialReadChan <- string(ibuf[0:n])
		}
	}()
	// Separate writer for character pacing
	go func() {
		obuf := make([]byte, 1)
		for {
			select {
			case b := <-serialWriteChan:
				obuf[0] = b
				_, err := c.Write(obuf)
				if err != nil {
					quitChan <- fmt.Sprintf("Error writing to %s: %v", device, err)
				}
				// Pace characters so we don't overwhelm the receive buffer
				time.Sleep((10000000 / time.Duration(consoleSpeed)) * time.Microsecond)
			}
		}
	}()
	return func() {
		for {
			select {
			case k := <-consoleInputChan:
				serialWriteChan <- byte(k)
			case s := <-serialReadChan:
				consoleOutputChan <- s
			}
		}
	}
}
