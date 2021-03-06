package attempt3

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"errors"

	"github.com/datacharmer/dbdeployer/common"

	"github.com/rogpeppe/go-internal/testscript"
)

var dryRun bool

func TestAttempt3(t *testing.T) {
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
				Dir:       dir,
				Cmds:      customCommands(),
				Condition: customConditions,
			})
		})
	}
}

func TestMain(m *testing.M) {
	flag.BoolVar(&dryRun, "dry", false, "creates testdata without running tests")
	versions := []string{"5.6.41", "5.7.31", "8.0.29"}

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

// fileExists reports whether a given file exists
func fileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return !errors.Is(err, fs.ErrNotExist)
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

// checkPorts is a testscript command that checks that the sandbox ports are as expected
func checkPorts(ts *testscript.TestScript, neg bool, args []string) {

	// portAdjustment80 defines the number of additional ports that should be expected for version 8.0
	portAdjustment80 := map[string]int{
		"single":               1,
		"master-slave":         3,
		"multiple":             3,
		"group-multi-primary":  3,
		"group-single-primary": 3,
	}
	if len(args) < 2 {
		ts.Fatalf("no sandbox path and number of ports provided")
	}
	sbDir := args[0]
	numPorts, err := strconv.Atoi(args[1])
	if err != nil {
		ts.Fatalf("error converting text '%s' to number: %s", args[1], err)
	}

	sbDescription, err := common.ReadSandboxDescription(sbDir)
	if err != nil {
		ts.Fatalf("error reading description file from %s: %s", sbDir, err)
	}
	isGreater, err := common.GreaterOrEqualVersion(sbDescription.Version, []int{8, 0, 1})
	if err != nil {
		ts.Fatalf("error comparing version '%s': %s", sbDescription.Version, err)
	}
	if isGreater {
		morePorts, ok := portAdjustment80[sbDescription.SBType]
		if !ok {
			ts.Fatalf("error recognizing the type of sandbox '%s': %s", path.Base(sbDir), sbDescription.SBType)
		}
		numPorts += morePorts
	}
	if len(sbDescription.Port) != numPorts {
		ts.Fatalf("sandbox '%s': wanted %d ports - got %d", path.Base(sbDir), numPorts, len(sbDescription.Port))
	}

}

// findErrorsInLogFile is a testscript command that finds ERROR strings inside a sandbox data directory
func findErrorsInLogFile(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) < 1 {
		ts.Fatalf("no sandbox path provided")
	}
	sbDir := args[0]
	dataDir := path.Join(sbDir, "data")
	logFile := path.Join(dataDir, "msandbox.err")
	if !dirExists(dataDir) {
		ts.Fatalf("sandbox data dir %s not found", dataDir)
	}
	if !fileExists(logFile) {
		ts.Fatalf("file %s not found", logFile)
	}

	contents, err := ioutil.ReadFile(logFile)
	if err != nil {
		ts.Fatalf("%s", err)
	}
	hasError := strings.Contains(string(contents), "ERROR")
	if neg && hasError {
		ts.Fatalf("ERRORs found in %s\n", logFile)
	}
	if !neg && !hasError {
		ts.Fatalf("ERRORs not found in %s\n", logFile)
	}
}

// checkFile is a testscript command that checks the existence of a list of files
// inside a directory
func checkFile(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) < 1 {
		ts.Fatalf("no sandbox path provided")
	}
	sbDir := args[0]

	for i := 1; i < len(args); i++ {
		f := path.Join(sbDir, args[i])
		exists := fileExists(f)

		if neg && exists {
			ts.Fatalf("file %s found", f)
		}
		if !exists {
			ts.Fatalf("file %s not found", f)
		}
	}
}

// sleep is a testscript command that pauses the execution for the required number of seconds
func sleep(ts *testscript.TestScript, neg bool, args []string) {
	duration := 0
	var err error
	if len(args) == 0 {
		duration = 1
	} else {
		duration, err = strconv.Atoi(args[0])
		if err != nil {
			ts.Fatalf("invalid number provided: '%s'", args[0])
		}
	}
	time.Sleep(time.Duration(duration) * time.Second)
}

func customConditions(condition string) (bool, error) {
	elements := strings.Split(condition, ":")
	if len(elements) == 0 {
		return false, fmt.Errorf("no condition found")
	}
	name := elements[0]
	switch name {
	case "minimum_version_for_group":
		if len(elements) < 2 {
			return false, fmt.Errorf("condition 'minimum_version_for_group' requires a version")
		}
		version := elements[1]
		if strings.HasPrefix(version, "5.7") || strings.HasPrefix(version, "8.0") {
			return true, nil
		}
		return false, nil

	case "exists_within_seconds":
		if len(elements) < 3 {
			return false, fmt.Errorf("condition 'exists_within_seconds' requires a file name and the number of seconds")
		}
		fileName := elements[1]
		delay, err := strconv.Atoi(elements[2])
		if err != nil {
			return false, err
		}
		if delay == 0 {
			return fileExists(fileName), nil
		}
		elapsed := 0
		for elapsed < delay {
			time.Sleep(time.Second)
			if fileExists(fileName) {
				return true, nil
			}
			elapsed++
		}
		return false, nil

	default:
		return false, fmt.Errorf("unrecognized condition name '%s'", name)

	}
}

func customCommands() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		// find_errors will check that the error log in a sandbox contains the string ERROR
		// invoke as "find_errors /path/to/sandbox"
		// The command can be negated, i.e. it will succeed if the log does not contain the string ERROR
		// "! find_errors /path/to/sandbox"
		"find_errors": findErrorsInLogFile,

		// check_file will check that a given list of files exists
		// invoke as "check_file /path/to/sandbox file1 [file2 [file3 [file4]]]"
		// The command can be negated, i.e. it will succeed if the given files do not exist
		// "! check_file /path/to/sandbox file1 [file2 [file3 [file4]]]"
		"check_file": checkFile,

		// sleep will pause execution for the required number of seconds
		// Invoke as "sleep 3"
		// If no number is passed, it pauses for 1 second
		"sleep": sleep,

		// check_ports will check that the number of ports expected for a given sandbox correspond to the ones
		// found in sbdescription.json
		// Invoke as "check_ports /path/to/sandbox 3"
		"check_ports": checkPorts,
	}
}
