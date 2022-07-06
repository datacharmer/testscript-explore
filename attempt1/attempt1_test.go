package attempt1

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestAttempt1(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
	})
}
