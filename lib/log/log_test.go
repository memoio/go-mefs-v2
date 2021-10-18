package log

import "testing"

func TestLog(t *testing.T) {
	testLog := Logger("test")
	testLog.Info("this is test")

	abcLog := Logger("abc")
	abcLog.Info("this is abc")

	testLog.Debug("this is test debug")
	testLog.Info("this is test info")
	testLog.Warn("this is test warn")
	testLog.Error("this is test error")
}
