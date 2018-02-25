// +build windows

package clipboardsignal

import (
	"testing"

	"github.com/atotto/clipboard"
)

func TestNotify(t *testing.T) {
	text, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll fails: %v", err)
	}
	defer clipboard.WriteAll(text)

	c := make(chan string, 1)
	Notify(c)
	defer Stop(c)

	const clipboardData = "hello hello hello"
	err = clipboard.WriteAll(clipboardData)
	if err != nil {
		t.Fatalf("WriteAll fails: %v", err)
	}

	if <-c != clipboardData {
		t.Fatal("the actual clipboard data and expected data do not match")
	}
}
