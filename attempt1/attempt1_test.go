package attempt1

import (
	"github.com/rogpeppe/go-internal/testscript"
	"testing"
)

func TestIntro(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
	})
}
