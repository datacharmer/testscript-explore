package attempt1

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"errors"

	"github.com/rogpeppe/go-internal/testscript"
)

var dryRun bool

func TestAttempt2(t *testing.T) {
	if dryRun {
		t.Skip("Dry Run")
	}
	// Directories in testdata are created by the setup code in TestMain
	dirs, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Skip("no directories found in testdata")
	}
	for _, dir := range dirs {
		t.Run(path.Base(dir), func(t *testing.T) {
			testscript.Run(t, testscript.Params{
				Dir: dir,
			})
		})
	}
}

func TestMain(m *testing.M) {
	flag.BoolVar(&dryRun, "dry", false, "creates testdata without running tests")
	versions := []string{"5.6.41", "5.7.30", "8.0.29"}

	for _, v := range versions {
		label := strings.Replace(v, ".", "_", -1)
		err := buildTests("templates", "testdata", label, map[string]string{
			"DbVersion": v,
			"DbPathVer": label,
			"Home":      os.Getenv("HOME"),
			"TmpDir":    "/tmp",
		})
		if err != nil {
			fmt.Printf("error creating the tests for %s :%s\n", label, err)
			os.Exit(1)
		}
	}
	exitCode := m.Run()

	if dirExists("testdata") && !dryRun {
		_ = os.RemoveAll("testdata")
	}
	os.Exit(exitCode)
}

// dirExists reports whether a given directory exists
func dirExists(filename string) bool {
	f, err := os.Stat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	fileMode := f.Mode()
	return fileMode.IsDir()
}

// buildTests takes all the files from templateDir and populates several data directories
// Each directory is named with the combination of the bare name of the template file + the label
// for example, from the data directory "testdata", file "single.tmpl", and label "8_0_29" we get the file
// "single_8_0_29.txt" under "testdata/8_0_29"
func buildTests(templateDir, dataDir, label string, data map[string]string) error {

	for _, needed := range []string{"DbVersion", "DbPathVer", "Home", "TmpDir"} {
		neededTxt, ok := data[needed]
		if !ok {
			return fmt.Errorf("[buildTests] the data must contain a '%s' element", needed)
		}
		if neededTxt == "" {
			return fmt.Errorf("[buildTests] the element '%s' in data is empty", needed)
		}
	}

	homeDir := data["Home"]
	if !dirExists(homeDir) {
		return fmt.Errorf("[buildTests] home directory '%s' not found", homeDir)
	}

	tmpDir := data["TmpDir"]
	if !dirExists(tmpDir) {
		return fmt.Errorf("[buildTests] temp directory '%s' not found", tmpDir)
	}

	if !dirExists(dataDir) {
		err := os.Mkdir(dataDir, 0755)
		if err != nil {
			return fmt.Errorf("[buildTests] error creating directory %s: %s", dataDir, err)
		}
	}
	files, err := filepath.Glob(templateDir + "/*.tmpl")

	if err != nil {
		return fmt.Errorf("[buildTests] error retrieving template files: %s", err)
	}
	for _, f := range files {
		fName := strings.Replace(path.Base(f), ".tmpl", "", 1)

		contents, err := ioutil.ReadFile(f)
		if err != nil {
			return fmt.Errorf("[buildTests] error reading file %s: %s", f, err)
		}

		subDataDir := path.Join(dataDir, label)
		if !dirExists(subDataDir) {
			err := os.Mkdir(subDataDir, 0755)
			if err != nil {
				return fmt.Errorf("[buildTests] error creating directory %s: %s", subDataDir, err)
			}
		}
		processTemplate := template.Must(template.New(label).Parse(string(contents)))
		buf := &bytes.Buffer{}

		if err := processTemplate.Execute(buf, data); err != nil {
			return fmt.Errorf("[buildTests] error processing template from %s: %s", f, err)
		}
		testName := path.Join(subDataDir, fName+"_"+label+".txt")
		err = ioutil.WriteFile(testName, buf.Bytes(), 0644)
		if err != nil {
			return fmt.Errorf("[buildTests] error writing text file %s: %s", testName, err)
		}

	}
	return nil
}
