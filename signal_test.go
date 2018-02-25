// +build windows

package clipboardsignal

import (
	"testing"

	"github.com/atotto/clipboard"
)

func TestNotify(t *testing.T) {
	text, err := clipboard.ReadAll()
	if err != nil {
		t.FailNow()
	}
	defer clipboard.WriteAll(text)

	c := make(chan Update, 1)
	Notify(c)
	defer Stop(c)

	const clipboardData = "hello hello hello"
	err = clipboard.WriteAll(clipboardData)
	if err != nil {
		t.FailNow()
	}

	if (<-c).Text != clipboardData {
		t.FailNow()
	}
}
