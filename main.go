package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const help_str = `
hpoon: Harpoon for the shell

Usage:
    hpoon <path> [name]         | store a mark, optionally with a name
    hpoon                       | retrieve the last marked path
    hpoon !<name>               | retrieve marked path with name (mark is recognized by prefix "!")
    hpoon list                  | list all named marked paths
    hpoon clean                 | delete all hpoon history

    can only mark files and directories that exist, but can retreive
    marks that no longer exist on the filesystem

Examples:

    cd /path/to/dir     # cd to a dir
    hpoon .             # harpoon it

    # in a different shell (ie: tmux)
    cd /new/abs/dir     # totally different dir
    cp * ` + "`hpoon`" + `      # copy files over to the last harpooned dir

    # works on deleted files
    hpoon filename myfile   #harpoon a file with "myfile"
    rm filename
    cd /somewhere/else/entirely
    mv some_file ` + "`hpoon !myfile`" + `   # result: mv some_file /original/path/filename
`

const short_help = "Unsure arg, try -h to get usage information"

const (
	LAST_MARKED_KEY = "_"
	KV_SEPERATOR    = "/"
	NAME_REF        = "!"
)

func quit(msg string, printargs ...any) {
	fmt.Printf(msg+"\n", printargs...)
	os.Exit(1)
}

func exit() {
	os.Exit(0)
}

func err(msg string, printargs ...any) error {
	return fmt.Errorf(msg, printargs...)
}

func report(printargs ...string) {
	fmt.Fprintln(os.Stderr, printargs)
}

type HarpoonRecord struct {
	last_marked string
	marks       map[string]string
}

func get_hpoon_file() string {
	switch runtime.GOOS {
	case "windows":
		return "C:\\Windows\\Temp\\hpoon"
	default:
		return "/tmp/hpoon"
	}
}

func parse_hpoon_line(line string) (string, string, error) {
	parts := strings.Split(line, KV_SEPERATOR)
	if len(parts) != 2 {
		return "", "", err("Invalid format of line: %s", line)
	}
	key := parts[0]
	value := parts[1]

	// decode value to preserve filepath oddities
	decoded_bytes, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", "", err
	}

	return key, string(decoded_bytes), nil
}

func create_hpoon_line(key string, value string) string {
	encoded_string := base64.StdEncoding.EncodeToString([]byte(value))

	return fmt.Sprintf("%s%s%s", key, KV_SEPERATOR, encoded_string)
}

func read_hpoon_file(filename string) HarpoonRecord {
	file, err := os.Open(filename)
	if err != nil {
		quit("Error reading hpoon marks file '%s'", filename)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	data := make(map[string]string)
	last_marked := ""

	for scanner.Scan() {
		line := scanner.Text()
		key, val, err := parse_hpoon_line(line)
		if err != nil {
			report("Failed to hpoon marks file", err.Error())
			continue
		}
		if key == LAST_MARKED_KEY {
			last_marked = val
		} else {
			data[key] = val
		}
	}

	if err := scanner.Err(); err != nil {
		file.Close()
		quit("Error parsing file:", err)
	}

	return HarpoonRecord{
		last_marked: last_marked,
		marks:       data,
	}
}

func write_hpoon_file(data HarpoonRecord, filename string) {
	file, err := os.Create(filename)
	if err != nil {
		quit("Error opening hpoon marks file '%s' reason: %s", filename, err.Error())
	}
	defer file.Close()

	non_value := func(v string) bool {
		return v == ""
	}

	if non_value(data.last_marked) {
		// we don't have data to write, so just return
		return
	}

	last_marked_line := create_hpoon_line(LAST_MARKED_KEY, data.last_marked)

	_, err = file.WriteString(last_marked_line + "\n")

	check_err := func(line string) {
		if err != nil {
			quit("Error writing to hpoon file: '%s' reason: '%s'", line, err.Error())
		}
	}

	check_err(last_marked_line)

	for key, value := range data.marks {
		if non_value(value) {
			continue
		}
		line := create_hpoon_line(key, value)
		_, err = file.WriteString(line + "\n")
		check_err(line)
	}
}

func load_hpoon() HarpoonRecord {
	hpoon_file := get_hpoon_file()
	if !check_path_exists(hpoon_file) {
		os.Create(hpoon_file)
	}

	return read_hpoon_file(hpoon_file)
}

func save_hpoon(data HarpoonRecord) {
	write_hpoon_file(data, get_hpoon_file())
}

func run_no_arg() {
	mark, err := hpoon_get_mark(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// no arg given, print the last given harpooned file
	fmt.Print(*mark)
}

func check_path_exists(fpath string) bool {
	_, err := os.Stat(fpath)
	return !os.IsNotExist(err)
}

func hpoon_set_mark(fpath string, name *string) {
	data := load_hpoon()
	data.last_marked = fpath
	if name != nil {
		data.marks[*name] = fpath
	}
	save_hpoon(data)
}

func hpoon_get_mark(name *string) (*string, error) {
	data := load_hpoon()
	if name == nil {
		return &data.last_marked, nil
	}
	value, exists := data.marks[*name]
	if !exists {
		return nil, fmt.Errorf("mark '%s' does not exist", *name)
	}
	return &value, nil
}

func check_name_ref(arg string) bool {
	return strings.HasPrefix(arg, NAME_REF)
}

func hpoon_out_mark_at(arg string) {
	name := arg[len(NAME_REF):]
	mark, err := hpoon_get_mark(&name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(*mark)
}

func hpoon_list() {
	record := load_hpoon()
	output := ""
	for mark, path := range record.marks {
		output = output + fmt.Sprintf("%s: %s\n", mark, path)
	}
	fmt.Print(output)
}

func hpoon_clean() {
	os.Remove(get_hpoon_file())
}

func run_single_arg(arg string) {
	switch arg {
	case "-h", "--help":
		fmt.Print(help_str)
	case "clean":
		hpoon_clean()
	case "list":
		hpoon_list()
	default:
		// we check if it's a name
		if check_name_ref(arg) {
			hpoon_out_mark_at(arg)
			return
		}
		// we check if it's a path
		path, err := filepath.Abs(arg)
		if err != nil {
			quit("Not sure what to do with: '%s'", arg)
		}

		if check_path_exists(path) {
			hpoon_set_mark(path, nil)
			return
		}
		// we abort, as we don't know what to do
		quit("Filepath doesn't exist: '%s'", path)
	}
}

func run_double_arg(arg string, name string) {
	// path, err := filepath.Abs(arg)
	// if err != nil {
	// 	quit("Not sure how to expand '%s'", arg)
	// }
	if !check_path_exists(arg) {
		quit("Filepath doesn't exist: '%s'", arg)
	}
	if arg == "." {
		cwd, _ := os.Getwd()
		arg = cwd
	}
	hpoon_set_mark(arg, &name)
}

func main() {
	switch len(os.Args) {
	case 1:
		run_no_arg()
	case 2:
		run_single_arg(os.Args[1])
	case 3:
		run_double_arg(os.Args[1], os.Args[2])
	default:
		quit(short_help)
	}
}
