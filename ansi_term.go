package main

// ANSI terminal emulation stuff

// ANSI is more or less close to the VT100 protocol

// references:
// http://www.termsys.demon.co.uk/vtansi.htm
// http://ascii-table.com/ansi-escape-sequences.php
// https://en.wikipedia.org/wiki/ANSI_escape_code
// https://www.tldp.org/HOWTO/Bash-Prompt-HOWTO/c327.html
// https://ispltd.org/mini_howto:ansi_terminal_codes
// IEEE 1275-1994


import (
	"fmt"
	"github.com/mgcaret/goncurses"
	"strconv"
	"strings"
)

const (
	norm = iota
	csi0
	csi1
)

// foreground color translation
// note that -1 is "terminal default" and may not be supported
// if it is not, it is replaced by C_WHITE
var colorTransFG = [9]int16{
	goncurses.C_BLACK,
	goncurses.C_RED,
	goncurses.C_GREEN,
	goncurses.C_YELLOW,
	goncurses.C_BLUE,
	goncurses.C_MAGENTA,
	goncurses.C_CYAN,
	goncurses.C_WHITE,
	-1,
}

// background color translation
// note that -1 is "terminal default" and may not be supported
// if it is not, it is replaced by C_BLACK
var colorTransBG = [9]int16{
	goncurses.C_BLACK,
	goncurses.C_RED,
	goncurses.C_GREEN,
	goncurses.C_YELLOW,
	goncurses.C_BLUE,
	goncurses.C_MAGENTA,
	goncurses.C_CYAN,
	goncurses.C_WHITE,
	-1,
}

var (
	curFGcolor    = 8              // current ANSI foreground color
	curBGcolor    = 8              // current ANSI background color)
	ansiState     = norm           // current ANSI state
	ansiParms     map[int]int      // ANSI escape sequence parameters
	ansiParmCount int              // ANSI escape parameter count
	ansiEaten     string      = "" // ANSI parameter accumulated chars
	savex         = 0			   // Saved cursor position X
	savey         = 0              // Saved cursor position Y
)

// Set up the color mapping from ANSI colors to Curses colors
func makeColors() {
	for b := 0; b < len(colorTransBG); b++ {
		for f := 0; f < len(colorTransFG); f++ {
			index := int16(b<<4 | f)
			goncurses.InitPair(index, colorTransFG[f], colorTransBG[b])
		}
	}
}

// ANSI terminal emulation to the Console window
func consoleWriteAnsi(args ...interface{}) {
	for _, arg := range args {
		str := fmt.Sprint(arg)
		a := []rune(str)
		for _, c := range a {
			consoleAnsi(c)
		}
	}
	consoleWindow.Refresh()
	if activeWindow != consoleWindow {
		fixCursor()
	}
}

// Write a character to the display, using the current colors if able
func consoleColorAddChar(c rune) {
	if goncurses.HasColors() {
		color := goncurses.ColorPair(int16(curBGcolor<<4 | curFGcolor))
		consoleWindow.AddChar(goncurses.Char(c) | color)
	} else {
		consoleWindow.AddChar(goncurses.Char(c))
	}
}

// This proceses an ANSI sequence parameter
func procAnsiParm() {
	if ansiEaten == "" {
		// No parameter given
		ansiParmCount++
	} else {
		// Should be numeric...
		parm, _ := strconv.Atoi(ansiEaten)
		ansiEaten = ""
		ansiParms[ansiParmCount] = parm
		ansiParmCount++
	}
}

// This sets up default ANSI parameters for a sequence
// if the sequence is missing them
func defAnsiParms(args ...int) {
	for i, parm := range args {
		if _, ok := ansiParms[i]; !ok {
			ansiParms[i] = parm
		}
	}
}

// This is the ANSI terminal state machine
func consoleAnsi(c rune) {
	switch ansiState {
	case norm:
		switch c {
		case 0x07: // BEL
			goncurses.Beep()
		case 0x08: // BS
			ansiCurRel(0, -1)
		case 0x09: // TAB
			curY, curX := consoleWindow.CursorYX()
			consoleWindow.Move(curY, (curX+8) & ^0x07)
		case 0x0A: // LF
			curY, _ := consoleWindow.CursorYX()
			maxY, _ := consoleWindow.MaxYX()
			if curY >= maxY-1 {
				consoleWindow.Scroll(1)
			} else {
				ansiCurRel(1, 0)
			}
		case 0x0B: // VT (reverse LF)
			ansiCurRel(-1, 0)
		case 0x0C: // FF
			consoleWindow.Erase()
			consoleWindow.Move(0, 0)
		case 0x0D: // CR
			curY, _ := consoleWindow.CursorYX()
			consoleWindow.Move(curY, 0)
		case 0x1B: // ESC
			ansiState = csi0
		default:
			consoleColorAddChar(c)
		}
	case csi0:
		switch c {
		case '[':
			ansiState = csi1
			ansiParms = make(map[int]int)
			ansiParmCount = 0
			ansiEaten = ""
		case 'c':
			ansiState = norm
			consoleWindow.AttrSet(0)
			consoleWindow.Erase()
			consoleWindow.Move(0,0)
			consoleWindow.Refresh()
		default:
			ansiState = norm
			consoleColorAddChar(0x1B)
			consoleColorAddChar(c)
		}
	case csi1:
		if c >= '0' && c <= '9' {
			// parameter
			ansiEaten += string(c)
		} else if c == ';' {
			// parameter separator
			procAnsiParm()
		} else {
			if len(ansiEaten) > 0 {
				procAnsiParm()
			}
			switch c {
			case 'A': // CUU
				defAnsiParms(1)
				ansiCUU()
			case 'B': // CUD
				defAnsiParms(1)
				ansiCUD()
			case 'C': // CUF
				defAnsiParms(1)
				ansiCUF()
			case 'D': // CUB
				defAnsiParms(1)
				ansiCUB()
			case 'E': // CNL
				defAnsiParms(1)
				ansiCNL()
			case 'F': // CPL
				defAnsiParms(1)
				ansiCPL()
			case 'G': // CHA
				defAnsiParms(1)
				ansiCHA()
			case 'H', 'f': // CUP
				defAnsiParms(1, 1)
				ansiCUP()
			case 'J': // ED
				defAnsiParms(0)
				ansiED()
			case 'K': // EL
				defAnsiParms(0)
				ansiEL()
			case 'L': // IL
				defAnsiParms(1)
				ansiIL()
			case 'M': // DL
				defAnsiParms(1)
				ansiDL()
			case 'P': // DC
				defAnsiParms(1)
				ansiDC()
			case '@': // IC
				defAnsiParms(1)
				ansiIC()
			case 'S': // SU
				defAnsiParms(1)
				ansiSU()
			case 'T': // SD
				defAnsiParms(1)
				ansiSD()
			case 'm': // SGR
				ansiSGR()
			case 'n': // DSR
				defAnsiParms(6)
				ansiDSR()
			case 'p': // Normal colors
				consoleWindow.AttrOff(goncurses.A_REVERSE)
			case 'q': // Inverse colors
				consoleWindow.AttrOn(goncurses.A_REVERSE)
			case 's': // Reset display (some sources), save cursor position (ANSI)
				// consoleWindow.AttrSet(0)
				savey, savex = consoleWindow.CursorYX()
			case 'u': // restore cursor position, see comment for 's'
				consoleWindow.Move(savey, savex)
			default:
				// nothing
			}
			ansiState = norm
			consoleWindow.Refresh()
		}
	default:
		ansiState = norm
		consoleColorAddChar(c)
	}
}

// Move the cursor relative to its current position
func ansiCurRel(ry int, rx int) {
	maxY, maxX := consoleWindow.MaxYX()
	curY, curX := consoleWindow.CursorYX()
	newY := curY + ry
	newX := curX + rx
	if newY < 0 {
		newY = 0
	}
	if newY >= maxY {
		newY = maxY - 1
	}
	if newX < 0 {
		newX = 0
	}
	if newX >= maxX {
		newX = maxX - 1
	}
	consoleWindow.Move(newY, newX)
}

// The next few routines are the handlers for various ANSI sequences

func ansiCUU() {
	ansiCurRel(-ansiParms[0], 0)
}

func ansiCUD() {
	ansiCurRel(ansiParms[0], 0)
}

func ansiCUF() {
	ansiCurRel(0, ansiParms[0])
}

func ansiCUB() {
	ansiCurRel(0, -ansiParms[0])
}

func ansiCNL() {
	_, curX := consoleWindow.CursorYX()
	ansiCurRel(ansiParms[0], -curX)
}

func ansiCPL() {
	_, curX := consoleWindow.CursorYX()
	ansiCurRel(-ansiParms[0], -curX)
}

func ansiCHA() {
	curY, _ := consoleWindow.CursorYX()
	consoleWindow.Move(curY, ansiParms[0]-1)
}

func ansiCUP() {
	consoleWindow.Move(ansiParms[0]-1, ansiParms[1]-1)
}

func ansiED() {
	curY, curX := consoleWindow.CursorYX()
	switch ansiParms[0] {
	case 0: // from cursor to bottom
		consoleWindow.ClearToBottom()
	case 1: // from beginning of screen to cursor
		for y := 0; y < curY; y++ {
			consoleWindow.Move(y, 0)
			consoleWindow.ClearToEOL()
		}
		consoleWindow.Move(curY, curX)
		ansiParms[0] = 1
		ansiEL()
	case 2, 3: // entire screen
		consoleWindow.Erase()
	}
	consoleWindow.Move(curY, curX)
}

func ansiEL() {
	curY, curX := consoleWindow.CursorYX()
	switch ansiParms[0] {
	case 0: // clear to end of line
		consoleWindow.ClearToEOL()
	case 1: // clear to beginning of line
		consoleWindow.Move(curY, 0)
		consoleWindow.Print(strings.Repeat(" ", curX))
	case 2: // clear whole line
		consoleWindow.Move(curY, 0)
		consoleWindow.ClearToEOL()
	}
	consoleWindow.Move(curY, curX)
}

// scroll up
func ansiSU() {
	consoleWindow.Scroll(ansiParms[0])
}

// scroll down
func ansiSD() {
	consoleWindow.Scroll(-ansiParms[0])
}

// delete line
func ansiDL() {
	consoleWindow.InsDelLine(-ansiParms[0])
}

// insert line
func ansiIL() {
	consoleWindow.InsDelLine(ansiParms[0])
}

func ansiDC() {
	for i := 0; i < ansiParms[0]; i++ {
		consoleWindow.DelChar()
	}
}

func ansiIC() {
	for i := 0; i < ansiParms[0]; i++ {
		consoleWindow.InsChar(' ')
	}
}

func ansiSGR() {
	for _, v := range ansiParms {
		switch v {
		case 0: // All normal
			consoleWindow.AttrSet(0)
		case 1: // Bold
			consoleWindow.AttrOn(goncurses.A_BOLD)
		case 2: // Faint
			consoleWindow.AttrOn(goncurses.A_DIM)
		case 4: // Underline
			consoleWindow.AttrOn(goncurses.A_UNDERLINE)
		case 5, 6: // Blink, Fast Blink
			consoleWindow.AttrOn(goncurses.A_BLINK)
		case 7: // Reverse
			consoleWindow.AttrOn(goncurses.A_REVERSE)
		case 8: // Conceal
			consoleWindow.AttrOn(goncurses.A_INVIS)
		case 10: // Default font
			consoleWindow.AttrOff(goncurses.A_ALTCHARSET)
		case 11, 12, 13, 14, 15, 16, 17, 18, 19: // Alternate font
			consoleWindow.AttrOn(goncurses.A_ALTCHARSET)
		case 21: // Bold off or double underline
			consoleWindow.AttrOff(goncurses.A_BOLD)
		case 22: // Normal intensity
			consoleWindow.AttrOff(goncurses.A_DIM)
		case 24: // Underline off
			consoleWindow.AttrOff(goncurses.A_UNDERLINE)
		case 25, 26: // Blink, Fast Blink off
			consoleWindow.AttrOff(goncurses.A_BLINK)
		case 27: // Reverse off
			consoleWindow.AttrOff(goncurses.A_REVERSE)
		case 28: // Conceal off
			consoleWindow.AttrOff(goncurses.A_INVIS)
		case 30, 31, 32, 33, 34, 35, 36, 37:
			curFGcolor = v - 30
		case 39:
			curFGcolor = 8
		case 40, 41, 42, 43, 44, 45, 46, 47:
			curBGcolor = v - 40
		case 49:
			curBGcolor = 8
		default:
			// Probably lots of TODO
		}
	}
}

func ansiDSR() {
	reply := ""
	switch ansiParms[0] {
	case 5: // Query device status
		reply = fmt.Sprint("\x1B[0n") // we are always happy
	case 6: // Query cursor position
		curY, curX := consoleWindow.CursorYX()
		reply = fmt.Sprintf("\x1B[%v;%vR", curY+1, curX+1)
	}
	for _, b := range []byte(reply) {
		consoleInputChan <- goncurses.Key(b)
	}
}

