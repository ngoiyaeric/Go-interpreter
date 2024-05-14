package witty

// Copyright 2021 The TCell Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
)

// stdIoTty is an implementation of the Tty API based upon stdin/stdout.
// We modify the original tcell implementation to make it possible to copy unprocessed stdin to
// the shell process.
type stdIoTty struct {
	fd     int
	in     *os.File
	out    *os.File
	saved  *term.State
	sig    chan os.Signal
	cb     func()
	stopQ  chan struct{}
	wg     sync.WaitGroup
	l      sync.Mutex
	mirror chan []byte
}

func (tty *stdIoTty) Read(b []byte) (int, error) {
	// log.Debug().Msgf("tty.Read()")
	n, err := tty.in.Read(b)
	if err != nil {
		log.Debug().Msgf("tty.Read() error: %v", err)
		return n, io.EOF
	}
	log.Debug().Msgf("tty.Read() - %d bytes", n)
	if tty.mirror != nil {
		log.Debug().Msgf("tty.Read() - mirroring %d bytes", n)
		tty.mirror <- b[:n]
	}
	log.Debug().Msgf("tty.Read() - done")
	return n, nil
}

func (tty *stdIoTty) Write(b []byte) (int, error) {
	return tty.out.Write(b)
}

func (tty *stdIoTty) Close() error {
	return nil
}

func (tty *stdIoTty) Start() error {
	log.Debug().Msgf("tty.Start()")
	tty.l.Lock()
	defer tty.l.Unlock()

	// We open another copy of /dev/tty.  This is a workaround for unusual behavior
	// observed in macOS, apparently caused when a subshell (for example) closes our
	// own tty device (when it exits for example).  Getting a fresh new one seems to
	// resolve the problem.  (We believe this is a bug in the macOS tty driver that
	// fails to account for dup() references to the same file before applying close()
	// related behaviors to the tty.)  We're also holding the original copy we opened
	// since closing that might have deleterious effects as well.  The upshot is that
	// we will have up to two separate file handles open on /dev/tty.  (Note that when
	// using stdin/stdout instead of /dev/tty this problem is not observed.)
	var err error
	tty.in = os.Stdin
	tty.out = os.Stdout
	tty.fd = int(tty.in.Fd())

	if !term.IsTerminal(tty.fd) {
		return errors.New("device is not a terminal")
	}

	err = tty.in.SetReadDeadline(time.Time{})
	if err != nil {
		log.Debug().Msgf("tty.Start() - SetReadDeadline() failed: %v", err)
	}
	err = syscall.SetNonblock(tty.fd, false)
	if err != nil {
		log.Debug().Msgf("tty.Start() - SetNonblock() failed: %v", err)
	}

	saved, err := term.MakeRaw(tty.fd) // also sets vMin and vTime
	if err != nil {
		return err
	}
	tty.saved = saved

	tty.stopQ = make(chan struct{})
	tty.wg.Add(1)
	go func(stopQ chan struct{}) {
		defer tty.wg.Done()
		for {
			select {
			case <-tty.sig:
				tty.l.Lock()
				cb := tty.cb
				tty.l.Unlock()
				if cb != nil {
					cb()
				}
			case <-stopQ:
				return
			}
		}
	}(tty.stopQ)

	signal.Notify(tty.sig, syscall.SIGWINCH)
	return nil
}

func (tty *stdIoTty) Drain() error {
	_ = tty.in.SetReadDeadline(time.Now())
	if err := tcSetBufParams(tty.fd, 0, 0); err != nil {
		return err
	}
	return nil
}

func (tty *stdIoTty) Stop() error {
	log.Debug().Msgf("tty.Stop()")
	tty.l.Lock()
	if err := term.Restore(tty.fd, tty.saved); err != nil {
		tty.l.Unlock()
		return err
	}
	_ = tty.in.SetReadDeadline(time.Now())

	signal.Stop(tty.sig)
	close(tty.stopQ)
	tty.l.Unlock()

	tty.wg.Wait()

	return nil
}

func (tty *stdIoTty) WindowSize() (int, int, error) {
	w, h, err := term.GetSize(tty.fd)
	if err != nil {
		return 0, 0, err
	}
	if w == 0 {
		w, _ = strconv.Atoi(os.Getenv("COLUMNS"))
	}
	if w == 0 {
		w = 80 // default
	}
	if h == 0 {
		h, _ = strconv.Atoi(os.Getenv("LINES"))
	}
	if h == 0 {
		h = 25 // default
	}
	return w, h, nil
}

func (tty *stdIoTty) NotifyResize(cb func()) {
	tty.l.Lock()
	tty.cb = cb
	tty.l.Unlock()
}

// NewMirrorTty creates a tty that mirrors its input to the given io.Writer
func NewMirrorTty(mirror chan []byte) (tcell.Tty, error) {
	tty := &stdIoTty{
		sig:    make(chan os.Signal),
		in:     os.Stdin,
		out:    os.Stdout,
		mirror: mirror,
	}
	var err error
	tty.fd = int(tty.in.Fd())
	if !term.IsTerminal(tty.fd) {
		return nil, errors.New("not a terminal")
	}
	if tty.saved, err = term.GetState(tty.fd); err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}
	return tty, nil
}
