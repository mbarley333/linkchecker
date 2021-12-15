package linkchecker

import (
	"context"
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

	ctx    context.Context
	cancel context.CancelFunc
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
		done:   make(chan struct{}),
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
	b.TotalSteps++
}

func (b *Bar) Completed() {

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.CompletedSteps++
}

func (b *Bar) GetPercent() float64 {

	b.mutex.RLock()
	defer b.mutex.RUnlock()

	var result float64

	if b.TotalSteps == 0 {
		result = 0
	} else {
		result = 100.0 * float64(b.CompletedSteps) / float64(b.TotalSteps)
	}

	return result

}

func (b *Bar) Render() {

	f := b.GetPercent()
	fmt.Fprintf(b.output, "%s%% complete        %d / %d\r", strconv.FormatFloat(f, 'f', 1, 64), b.CompletedSteps, b.TotalSteps)

}

// loop for refreshing the progressbar
func (b *Bar) Refresher() {
	for {
		select {
		// case <-b.done:
		// 	return
		case <-b.ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			b.Render()
		}
	}
}
