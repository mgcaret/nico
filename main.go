package main

import (
	"flag"
	"fmt"
	"github.com/mgcaret/goncurses"
	"log"
	"os"
)

const VERSION = "0.0.1"

type consoleIoServicerFunc func()
type debugInterfaceFunc func()

var (
	consoleInputChan = make(chan goncurses.Key, 16) // console input channel
	consoleOutputChan = make(chan string, 16)       // console output channel
	debugOutputChan = make(chan string, 16)         // console output channel
	debugCommandChan = make(chan []string, 16)      // debug command channel
	quitChan = make(chan string, 3)                 // quit channel (non-blocking)
	consoleIoServicer consoleIoServicerFunc         // Console I/O servicer routine
	debugInterface debugInterfaceFunc               // Debug I/O servicer routine
	exitReason string = ""                          // final message for user
	noDebug = false									// omit debug window if true
)

func init() {
	log.Println(
		"Neon816 Integrated Console -",
		fmt.Sprintf("nico v%s by Michael Guidero", VERSION))
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s <console-device> [<debug-device>]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.UintVar(&consoleSpeed, "console-baud", 9600, "Set console baud")
	flag.UintVar(&debugSpeed, "debug-baud", 57600, "Set console baud")
	flag.BoolVar(&noDebug, "no-debug", false, "Disable debug/command interface")
	flag.Parse()
}

// Get everything set up
func main() {
	testMode := false
	debugInterface = nullDebugInterface
	if consoleDevice := flag.Arg(0); consoleDevice != "" {
		if consoleDevice == "test" {
			testMode = true
			consoleIoServicer = demoConsoleIoServicer
			consoleOutputChan <- "*** Test Mode ***\r\n"
		} else {
			log.Printf("Console device: %s", consoleDevice)
			consoleIoServicer = getConsoleIoServicer(consoleDevice)
		}
	}
	if consoleIoServicer == nil {
		log.Fatal("Invalid or no console device specified!")
	}
	if noDebug {
		debugInterface = noDebugInterface
	} else {
		if debugDevice := flag.Arg(1); debugDevice != "" {
			log.Printf("Debug device: %s", debugDevice)
			debugInterface = getDebugInterface(debugDevice)
			if debugInterface == nil {
				debugInterface = nullDebugInterface
				log.Print("Debug device is not usable")
				debugOutputChan <- fmt.Sprintf("Debug device %s is not usable!\n", debugDevice)
			}
		}
	}
	src, err := goncurses.Init()
	if err != nil {
		log.Fatal("init:", err)
	}
	defer func() {
		if !goncurses.IsEnd() {
			goncurses.Raw(false)
			goncurses.End()
		}
		if exitReason != "" {
			log.Println(exitReason)
		}
	}()
	goncurses.NewLines(false)
	goncurses.Echo(false)
	goncurses.Raw(true)
	goncurses.StartColor()
	err = goncurses.UseDefaultColors()
	if err != nil {
		// assume white on black if we can't figure out the terminal
		// default colors
		colorTransFG[8] = goncurses.C_WHITE
		colorTransBG[8] = goncurses.C_BLACK
	}
	makeColors()
	winSetup(src)
	if testMode {
		maxY, maxX := consoleWindow.MaxYX()
		consoleWindow.Println(fmt.Sprintf("[console %vx%v]", maxX, maxY))
		if debugWindow != nil {
			maxY, maxX = debugWindow.MaxYX()
			debugWindow.Println(fmt.Sprintf("[debug %vx%v]", maxX, maxY))
		}
	}
	consoleWindow.Refresh()
	if debugWindow != nil {
		debugWindow.Refresh()
	}
	commandString = ""
	if commandInputWindow != nil {
		commandInputWindow.Refresh()
	}
	// at this point, nobody must make Curses calls outside of uiServicer
	mainApp()
	if !goncurses.IsEnd() {
		goncurses.Raw(false)
		goncurses.End()
	}
}

// Start all main goroutines and wait for one of them to tell us to quit
func mainApp() {
	consoleOutputChan <- fmt.Sprintf("nico v%s by Michael Guidero\r\n", VERSION)
	go consoleIoServicer()
	go debugInterface()
	go uiServicer() // in text_ui.go
	select {
	case exitReason = <-quitChan:
	}
}

// Sets up the Curses windows
func winSetup(src *goncurses.Window) {
	ysize, xsize := src.MaxYX()
	if noDebug {
		consoleWindow = src
	} else {
		wsplit := 25
		if ysize < 30 {
			wsplit = ysize - 5
		}
		src.HLine(wsplit, 0, goncurses.ACS_HLINE, xsize)
		consoleWindow = src.Derived(wsplit, xsize, 0, 0)
		debugWindow = src.Derived(ysize-wsplit-2, xsize, wsplit+1, 0)
		debugWindow.ScrollOk(true)
		debugWindow.Keypad(true)
		debugWindow.Timeout(0)
		commandInputWindow = src.Derived(1, xsize, ysize-1, 0)
		commandInputWindow.ScrollOk(false)
		commandInputWindow.Keypad(true)
		commandInputWindow.Timeout(50)
	}
	consoleWindow.ScrollOk(true)
	consoleWindow.Keypad(true)
	consoleWindow.Timeout(50)
	activeWindow = consoleWindow
	src.Refresh()
}

