package log

import (
	"fmt"
	"sync/atomic"
)

type Sink func(msg string)

var robotSink atomic.Pointer[Sink]

func SetRobotSink(sink Sink) {
	if sink == nil {
		robotSink.Store(nil)
		return
	}
	robotSink.Store(&sink)
}

func Robotf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if sink := robotSink.Load(); sink != nil {
		(*sink)(msg)
		return
	}
	fmt.Print(msg)
}
