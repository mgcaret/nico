package main

import (
	"errors"
	"fmt"
	"github.com/chrizzzzz/go-xmodem/xmodem"
	"github.com/chrizzzzz/go-xmodem/ymodem"
	"io/ioutil"
	"path/filepath"
	"time"
)

const (
	pAscii = iota
	pXmodem
	pXmodem1k
	pYmodem
)

type FileXfer struct {
	Cancel             chan bool
	FileXferInputChan  chan byte
	FileXferOutputChan chan byte
	cancelled          bool
}

func newFileXfer() *FileXfer {
	v := FileXfer{
		make(chan bool),
		make(chan byte, 2048),
		make(chan byte, 2048),
		false,
	}
	return &v
}

func xferLogByte(b byte) string {
	if b < 32 || b > 126 {
		return fmt.Sprintf("[0x%02X]", b)
	} else {
		return fmt.Sprintf("[0x%02X '%c']", b, b)
	}
}

func (rw FileXfer) Read(b []byte) (int, error) {
	bytes := 0
lp:
	for i := 0; i < len(b); i++ {
		select {
		case b[bytes] = <-rw.FileXferInputChan:
			debugOutputChan <- fmt.Sprintf("Xfer: Neon->Proto %v\n", xferLogByte(b[bytes]))
			bytes++
		case rw.cancelled = <-rw.Cancel:
			if rw.cancelled {
				break lp
			}
		case <-time.After(60 * time.Second):
			break lp
		}
	}
	err := error(nil)
	if rw.cancelled {
		err = errors.New("Transfer cancelled!")
	}
	return bytes, err
}

func (rw FileXfer) Write(p []byte) (int, error) {
lp:
	for _, b := range p {
		select {
		case rw.cancelled = <-rw.Cancel:
			if rw.cancelled {
				break lp
			}
		default:
			rw.FileXferOutputChan <- b
			debugOutputChan <- fmt.Sprintf("Xfer: Proto->Neon %v\n", xferLogByte(b))
		}
	}
	err := error(nil)
	if rw.cancelled {
		err = errors.New("Transfer cancelled!")
	}
	return len(p), err
}

func (rw FileXfer) sendAscii(data []byte) error {
lp:
	for _, b := range data {
		select {
		case rw.cancelled = <-rw.Cancel:
			if rw.cancelled {
				break lp
			}
		default:
			rw.FileXferOutputChan <- b
		}
	}
	err := error(nil)
	if rw.cancelled {
		err = errors.New("Transfer cancelled!")
	}
	return err
}

func (rw FileXfer) sendFile(proto int, name string) error {
	data, err := ioutil.ReadFile(name)
	debugOutputChan <- fmt.Sprintf("Sending %v, size %v...\n", name, len(data))
	if err != nil {
		return err
	}
	switch proto {
	case pAscii:
		err = rw.sendAscii(data)
	case pXmodem:
		err = xmodem.ModemSend(rw, data)
	case pXmodem1k:
		err = xmodem.ModemSend1K(rw, data)
	case pYmodem:
		err = ymodem.ModemSend(rw, data, filepath.Base(name))
	}
	return err
}
