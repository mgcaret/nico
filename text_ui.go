package main

import (
	"encoding/csv"
	"fmt"
	"github.com/rthornton128/goncurses"
	"strings"
)

var (
	debugWindow        *goncurses.Window // debug output window
	commandInputWindow *goncurses.Window // command input window
	commandString      string            // command string
	commandCursor      = 0               // cursor position in command string
	consoleWindow      *goncurses.Window // console window
	activeWindow       *goncurses.Window // current active input window
)

// All screen/keyboard I/O is done in this function, to be used as
// a goroutine, in order to make sure that we don't try to use Curses
// functions concurrently
func uiServicer() {
	for {
		select {
		case s := <-consoleOutputChan:
			consoleWriteAnsi(s)
		case s := <-debugOutputChan:
			debugWrite(s)
		default:
			k := activeWindow.GetChar()
			if k != 0 {
				serviceKey(k)
			}
		}
	}
}

// Routine to service all keypresses received in the UI
// regardless of the active Curses window.
func serviceKey(k goncurses.Key) {
	switch k {
	case 0:
		// nothing
	case 0x1B: // esc
		// Remember to not conflict with debug window editing
		// keystrokes when adding Alt+Key combos here!
		l := activeWindow.GetChar()
		switch l {
		case 0:
			consoleInputChan <- k
		case 9: // Alt+Tab
			swapWindow()
		case 32: // Alt+Space
			debugCommandChan <- []string{"step"}
		case 96: // Alt+`
			debugCommandChan <- []string{"reset"}
		case 103: // Alt+G
			debugCommandChan <- []string{"cont"}
		case 114: // Alt+R
			debugCommandChan <- []string{"run"}
		case 115: // Alt+S
			debugCommandChan <- []string{"stop"}
		default:
			switch activeWindow {
			case consoleWindow:
				debugWrite(fmt.Sprintf("[Alt+%s]", goncurses.KeyString(l)))
				consoleInputChan <- k
				consoleInputChan <- l
			case commandInputWindow:
				// Negatives probably don't conflict with anything important
				commandLineInput(-l)
			}
		}
	case 0x1D: // ^]
		// Keyboard command sequence
		if keyboardCommand() {
			quitChan <- "" // We quit if it returns true
		}
	case goncurses.KEY_F1:
		helpText()
	case goncurses.KEY_F2:
		swapWindow()
	case goncurses.KEY_F10:
		quitChan <- ""
	default:
		switch activeWindow {
		case consoleWindow:
			consoleInputChan <- translateKey(k)
		case commandInputWindow:
			commandLineInput(k)
		}
	}
}

// Key translation before sending down the wire
func translateKey(k goncurses.Key) goncurses.Key {
	switch k {
	case goncurses.KEY_BACKSPACE:
		return goncurses.Key(0x08)
	}
	return k
}

func imin(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func commandLineInput(k goncurses.Key) {
	l := commandString[0:commandCursor]
	r := ""
	if commandCursor < len(commandString) {
		r = commandString[commandCursor:]
	}
	if k < 32 || k > 126 {
		// some kind of control key
		switch k {
		case 13, 10, goncurses.KEY_ENTER, goncurses.KEY_CANCEL:
			switch k {
			case 13, 10, goncurses.KEY_ENTER:
				if len(commandString) > 0 {
					processCommandLine(commandString)
				}
				l = ""
				r = ""
			}
		case 12:
			if debugWindow != nil {
				debugWindow.Clear()
				debugWindow.Move(0, 0)
				debugWindow.Refresh()
			}
		case 127, goncurses.KEY_BACKSPACE:
			if len(l) > 0 {
				l = l[0 : len(l)-1]
			}
		case goncurses.KEY_LEFT, 8, 2:
			if len(l) > 0 {
				r = l[len(l)-1:] + r
				l = l[0 : len(l)-1]
				//debugWrite(fmt.Sprintf("|%s:%s\n", l, r))
			}
		case goncurses.KEY_RIGHT, 6:
			if len(r) > 0 {
				l = l + r[0:1]
				r = r[1:]
				//debugWrite(fmt.Sprintf("|%s:%s\n", l, r))
			}
		case goncurses.KEY_HOME, 1:
			r = l + r
			l = ""
		case 360, 5: // why no KEY_END, goncurses????
			l = l + r
			r = ""
		case 11:
			r = ""
		case goncurses.KEY_CLEAR, 24:
			if r != "" {
				r = ""
			} else {
				l = ""
			}
		case goncurses.KEY_DC, 4:
			if r != "" {
				r = r[1:]
			}
		case -98: // Alt+B - back word
			for l != "" && l[len(l)-1:] == " " {
				r = l[len(l)-1:] + r
				l = l[0 : len(l)-1]
			}
			for l != "" && l[len(l)-1:] != " " {
				r = l[len(l)-1:] + r
				l = l[0 : len(l)-1]
			}
		case -102: // Alt+F - forward word
			for r != "" && r[0:1] != " " {
				l = l + r[0:1]
				r = r[1:]
			}
			for r != "" && r[0:1] == " " {
				l = l + r[0:1]
				r = r[1:]
			}
		case 25, -13, -121: // ^Y, Alt+Enter, Alt+Y - yank into console window
			for _, b := range []byte(l + r) {
				consoleInputChan <- goncurses.Key(b)
			}
			// All except Alt+Y clear the debug input
			if k != -121 {
				l = ""
				r = ""
			}
		default:
			debugWrite(fmt.Sprintf("[%v]", k))
		}
	} else {
		// an input character
		l = l + string(k)
	}
	commandCursor = len(l)
	commandString = l + r
	commandInputWindow.Move(0, 0)
	_, maxX := commandInputWindow.MaxYX()
	if len(commandString) < maxX {
		commandInputWindow.Print(commandString)
		commandInputWindow.ClearToEOL()
		commandInputWindow.Move(0, commandCursor)
	} else {
		// sc = max chars on the right hand side, proportional to window size
		sc := maxX / 8
		// s = length of left hand side to draw
		// t = length of right hand side to draw
		t := imin(sc, len(r))
		s := maxX - t - 1
		if len(l) < maxX-sc {
			s = len(l)
			t = maxX - s
		}
		//debugWrite(fmt.Sprintf("%v|%v|%v|%v\n", len(l), s, t, len(r)))
		//command_input_window.AttrOn(goncurses.A_REVERSE)
		commandInputWindow.Print(l[len(l)-s:])
		//command_input_window.AttrOff(goncurses.A_REVERSE)
		commandInputWindow.Print(r[0:t])
		commandInputWindow.ClearToEOL()
		commandInputWindow.Move(0, s)
	}
	commandInputWindow.Refresh()
}

func processCommandLine(cl string) {
	ct := strings.Trim(cl, " ")
	if ct == "" {
		return
	}
	debugOutputChan <- fmt.Sprintf("> %s\n", ct)
	r := csv.NewReader(strings.NewReader(ct))
	r.Comma = ' '
	r.Comment = '#'
	words, err := r.Read()
	if err != nil {
		// We trim off the line and column from the error message since
		// there is only one line.
		errs := strings.SplitN(fmt.Sprintf("%v", err), ": ", 2)
		errn := len(errs) - 1
		debugOutputChan <- fmt.Sprintf("Command syntax error: %v!\n", errs[errn])
		return
	}
	switch strings.ToLower(words[0]) {
	case "quit":
		quitChan <- ""
	case "help":
		helpText()
	default:
		debugCommandChan <- words
	}
}

// This handles the keyboard command key (^[) functions
func keyboardCommand() bool {
	// Prompt for command
	_, mX := consoleWindow.MaxYX()
	save := consoleWindow.Derived(1, 8, 0, mX-8)
	save.Overwrite(consoleWindow)
	annun := save.Duplicate()
	annun.Erase()
	annun.AttrSet(goncurses.A_REVERSE)
	annun.Println("Command?")
	annun.Refresh()
	annun.Timeout(2000)
	l := annun.GetChar()
	annun.Erase()
	annun.Refresh()
	annun.Delete()
	save.Touch()
	save.Refresh()
	save.Delete()
	consoleWindow.TouchLine(0, 1)
	consoleWindow.Refresh()
	fixCursor()
	//goncurses.Update()
	switch l {
	case 9: // tab
		swapWindow()
	case 99, 67: // c, C
		consoleWindow.AttrSet(0)
		consoleWindow.Erase()
		consoleWindow.Move(0, 0)
		consoleWindow.Refresh()
	case 100, 68: // d, D
		if debugWindow != nil {
			debugWindow.AttrSet(0)
			debugWindow.Erase()
			debugWindow.Move(0, 0)
			debugWindow.Refresh()
		}
	case 104, 72: // h, H
		helpText()
	case 113, 81: // q, Q
		return true
	case 0x1B: // ESC
		// Nothing (cancel command without flash)
	case 0, 0x1D:
		//debugWrite("[CMD TO]")
		switch activeWindow {
		case consoleWindow:
			consoleInputChan <- 0x1D
		case commandInputWindow:
			// nothing
		}
	default:
		// Invalid command key
		goncurses.Flash()
	}
	return false
}

// Puts the help text in the debug window
func helpText() {
	debugOutputChan <- "Help: F1=help; F2 or alt+tab=swap console/debug; F10=quit, ^]=command\n"
	debugOutputChan <- "  commands: tab=swap, [c]lear console, clear [d]ebug, [h]elp, [q]uit\n"
}

// Swaps the active input window, note the debug input window is
// optional
func swapWindow() {
	if activeWindow == consoleWindow && commandInputWindow != nil {
		activeWindow = commandInputWindow
	} else {
		activeWindow = consoleWindow
	}
	fixCursor()
}

// Write text to the debug window, temporarily reactivates newlines
func debugWrite(args ...interface{}) {
	if debugWindow == nil {
		return
	}
	goncurses.NewLines(true)
	debugWindow.Print(args...)
	debugWindow.Refresh()
	goncurses.NewLines(false)
	if activeWindow != debugWindow {
		fixCursor()
	}
}

// this routine moves the cursor back to the active input window
// if we have done something that might have moved it.
func fixCursor() {
	curY, curX := activeWindow.CursorYX()
	activeWindow.Move(curY, curX)
}

// Basic write to the Console window
func consoleWrite(args ...interface{}) {
	consoleWindow.Print(args...)
	consoleWindow.Refresh()
	if activeWindow != consoleWindow {
		fixCursor()
	}
}
