package linkchecker

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

type Bar struct {
	TotalSteps     int
	CompletedSteps int

	Done chan bool

	mutex  sync.RWMutex
	output io.Writer
}

type OptionBar func(*Bar) error

func WithOutputBar(output io.Writer) OptionBar {
	return func(b *Bar) error {
		b.output = output
		return nil
	}
}

func NewBar(opts ...OptionBar) *Bar {
	bar := &Bar{
		Done:   make(chan bool),
		output: os.Stdout,
	}

	for _, o := range opts {
		o(bar)
	}

	return bar
}

func (b *Bar) Add() {

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.TotalSteps += 1
}

func (b *Bar) Completed() {

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.CompletedSteps += 1
}

func (b *Bar) GetPercent() float64 {

	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return 100.0 * float64(b.CompletedSteps) / float64(b.TotalSteps)

}

func (b *Bar) Render() {

	f := b.GetPercent()
	fmt.Fprintf(b.output, "\r%s%% completed", strconv.FormatFloat(f, 'f', 1, 64))

}

// loop for refreshing the progressbar
func (b *Bar) Refresher() {
	for {
		select {
		case b.Done <- true:
			return
		case <-time.After(1 * time.Second):
			fmt.Println("ggggggg")
			b.Render()
		}
	}
}
