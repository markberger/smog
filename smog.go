package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/howeyc/fsnotify"
	"github.com/kr/binarydist"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

var tahoe_url = "http://127.0.0.1:3456/"

type settings struct {
	URI string
}

type child struct {
	Name       string
	Type       string
	Verify_URI string
	RO_URI     string
	RW_URI     string
	Mutable    bool
	Linkmotime float64
	Linkcrtime float64
}

// These four declarations are necessary for sort

type Children []child

func (c Children) Len() int {
	return len(c)
}

func (c Children) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c Children) Less(i, j int) bool {
	return c[i].Name < c[j].Name
}

func createDir() settings {
	u := make(url.Values)
	resp, err := http.PostForm(tahoe_url+"uri?t=mkdir&format=mdmf", u)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	var s settings
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	s.URI = string(body)
	err = os.Mkdir(".smog/", 0744)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.OpenFile(".smog/settings", os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	out, err := json.Marshal(s)
	f.Write(out)
	os.Mkdir("./restored", 0777)
	return s
}

func initialize() settings {
	f, err := os.Open("./.smog/settings")
	if err != nil {
		if os.IsNotExist(err) {
			return createDir()
		} else {
			log.Fatal(err)
		}
	}
	defer f.Close()
	var s settings
	decoder := json.NewDecoder(f)
	err = decoder.Decode(&s)
	if err != nil {
		log.Fatal(err)
	}
	return s
}

func delete(dircap string, file string, db *_db) {
	db.update_renew(file, false)
	db.save()
	log.Printf("%s is deleted. Its leases will not be renewed.\n", file)
}

func move_to_hidden(file string) {
	_ = os.Remove(".smog/" + file)
	src, err := os.Open(file)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	defer src.Close()

	dst, err := os.Create(".smog/" + file)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	return
}

func tahoe_mkdir(dircap string, file string) string {
	folder := "file-" + file
	dest := tahoe_url + "uri/" + dircap + "/?t=mkdir&name=" + folder
	u := make(url.Values)
	resp, err := http.PostForm(dest, u)
	if err != nil {
		log.Printf("%v\n", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v\n", err)
	}
	return string(body)
}

func tahoe_renew_file(filecap string, name string) {
	l := tahoe_url + "uri/" + filecap + "/?t=start-deep-check&add-lease=true&ophandle=" + name
	u := make(url.Values)
	resp, err := http.PostForm(l, u)
	if err != nil {
		log.Printf("Encountered error during renew: %v\n", err)
	}
	if resp.StatusCode != 200 {
		log.Printf("Error: response code from renew is %d\n", resp.StatusCode)
	}
}

// Download a file from the tahoe grid.

func tahoe_dl(filecap string, dest string, file string) {
	fmt.Printf("%s\n", filecap)
	url := tahoe_url + "uri/" + filecap
	fmt.Printf("%s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(dest+file, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
}

// Get the linkmotime from a file on the grid.

func get_linkmotime(dircap string, file string) float64 {
	dest := tahoe_url + "uri/" + dircap + "/" + file + "?t=json"
	resp, err := http.Get(dest)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var raw []*json.RawMessage
	json.Unmarshal(body, &raw)
	var m map[string]*json.RawMessage
	json.Unmarshal(*raw[1], &m)
	json.Unmarshal(*m["metadata"], &m)
	var n map[string]float64
	json.Unmarshal(*m["tahoe"], &n)
	return n["linkmotime"]
}

// Get all of the children from a directory.

func get_children(uri string) []child {
	url := tahoe_url + "uri/" + uri + "/?t=json"
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("%v\n", err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var raw []*json.RawMessage
	json.Unmarshal(body, &raw)
	var m map[string]*json.RawMessage
	json.Unmarshal(*raw[1], &m)
	var raw_children map[string]*json.RawMessage
	json.Unmarshal(*m["children"], &raw_children)

	var children []child

	for key, val := range raw_children {
		json.Unmarshal(*val, &raw)
		json.Unmarshal(*raw[1], &m)

		c := new(child)
		json.Unmarshal(*raw[1], c)
		c.Name = key

		var metadata map[string]*json.RawMessage
		json.Unmarshal(*m["metadata"], &metadata)
		var tahoe map[string]float64
		json.Unmarshal(*metadata["tahoe"], &tahoe)

		c.Linkcrtime = tahoe["linkcrtime"]
		c.Linkmotime = tahoe["linkmotime"]
		children = append(children, *c)
	}

	return children
}

// Sort the children according to their base number and patch
// number. The index to the 2d slice is the base number of the
// desired version, and this returns a slice which has the base
// and its patches sorted by order of application (ie patch 1, then 2, etc.)

func sort_children(c []child) [][]child {
	// This is the worst case scenario and should be smarter
	bases := make([][]child, len(c))
	for i := range c {
		if strings.HasPrefix(c[i].Name, "base-") {
			index, _ := strconv.Atoi(c[i].Name[5:])
			bases[index] = append(bases[index], c[i])
		}
		if strings.HasPrefix(c[i].Name, "patch-") {
			str := c[i].Name[6:]
			nums := strings.Split(str, ".")
			base_num, _ := strconv.Atoi(nums[0])
			bases[base_num] = append(bases[base_num], c[i])
		}
	}

	for i := range bases {
		sort.Sort(Children(bases[i]))
	}

	return bases
}

func renew_all() {
	db := load_db()
	r := db.get_renewables()
	for i := range r {
		fmt.Printf("Renewing %s\n", r[i].Name)
		tahoe_renew_file(r[i].URI, r[i].Name)
	}
}

// Upload a file for the first time.

func new_upload(dircap string, file string, db *_db) {
	go move_to_hidden(file)
	uri := tahoe_mkdir(dircap, file)
	f, err := ioutil.ReadFile(file)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	reader := bytes.NewReader(f)
	folder := "file-" + file
	dest := tahoe_url + "uri/" + dircap + "/" + folder + "/base-0?format=chk"
	r, err := http.NewRequest("PUT", dest, reader)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	c := &http.Client{}
	resp, err := c.Do(r)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		log.Printf("Successfully created %s\n", file)
	} else {
		log.Printf("File was not created. Response code: %v\n", resp.StatusCode)
	}

	linkmotime := get_linkmotime(dircap, folder+"/base-0")
	db.add(file, 0, 0, linkmotime, uri)
	db.save()
}

// Generate the binary diff for a given file. It is assumed that
// the previous version of the file is in .smog

func generate_diff(file string) string {
	t := time.Now()
	name := file + t.String()
	patch, err := os.OpenFile(".smog/"+name, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer patch.Close()

	old, err := os.Open(".smog/" + file)
	if err != nil {
		log.Fatal(err)
	}
	defer old.Close()

	curr, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer curr.Close()

	err = binarydist.Diff(old, curr, patch)
	if err != nil {
		log.Fatal(err)
	}

	return name
}

// Generate a diff for the file and upload that diff with
// the appropriate patch number.

func upload(dircap string, file string, db *_db) {
	folder := "file-" + file + "/"
	patch_path := generate_diff(file)
	go move_to_hidden(file)
	f, err := ioutil.ReadFile(".smog/" + patch_path)
	if err != nil {
		log.Printf("%v\n", err)
	}
	reader := bytes.NewReader(f)
	base_num := strconv.Itoa(db.get_basenum(file))
	patch_num := db.get_patchnum(file)
	patch_num = patch_num + 1
	dest_name := "patch-" + base_num + "." + strconv.Itoa(patch_num)

	dest := tahoe_url + "uri/" + dircap + "/" + folder + dest_name + "?format=chk"
	r, err := http.NewRequest("PUT", dest, reader)
	if err != nil {
		log.Printf("%v\n", err)
	}
	c := &http.Client{}
	resp, err := c.Do(r)
	if err != nil {
		log.Printf("%v\n", err)
	}
	if resp.StatusCode == 200 {
		log.Printf("Error: Overwrote an existing patch.")
		log.Printf("File: %s  base: %s  patch: %d", file, base_num, patch_num)
	} else if resp.StatusCode == 201 {
		log.Printf("Successfully created %s %s\n", file, dest_name)
	}
	db.update_patchnum(file, patch_num)
	db.save()
	os.Remove(".smog/" + patch_path)
}

// Print the children nicely when a user is restoring their file.

func pretty_print_children(children [][]child) {
	fmt.Printf("\n\tSnapshot \t\tDate\n")
	for i := range children {
		for j := range children[i] {
			c := children[i][j]
			s := int64(c.Linkmotime)
			t := time.Unix(s, 0)
			const layout = "Jan 2, 2006 at 3:04pm (MST)"
			fmt.Printf("\t  %d.%d\t\t  %v\n", i, j, t.Format(layout))
		}
	}
	fmt.Printf("\n")
}

// Prompt the user for the version of the file they wish to restore.

func get_version(children [][]child) (int, int) {
	pretty_print_children(children)
	var base, patch int
	fmt.Printf("Restore snapshot: ")
	_, _ = fmt.Scanf("%d.%d", &base, &patch)
	return base, patch
}

func restore_file(file string, db *_db) bool {
	f, err := db.get_file(file)
	if err != nil {
		return false
	}
	children := get_children(f.URI)
	c := sort_children(children)
	base, patch := get_version(c)
	restore(file, c[base], patch)
	return true
}

func restore(file string, c []child, patch_num int) {
	os.MkdirAll("restored/.tmp", 0777)
	tahoe_dl(c[0].RO_URI, "restored/.tmp/", c[0].Name)
	for i := 1; i < patch_num+1; i++ {
		tahoe_dl(c[i].RO_URI, "restored/.tmp/", c[i].Name)
	}
	f_path := rebuild_file(c, patch_num, "restored/")
	err := os.Rename(f_path, "restored/"+file)
	if err != nil {
		log.Fatal(err)
	}
	os.RemoveAll("restored/.tmp/")
}

func rebuild_file(c []child, patch_num int, dest string) string {
	if patch_num == 0 {
		return "restored/.tmp/" + c[0].Name
	}

	err := os.Rename("restored/.tmp/"+c[0].Name, "restored/.tmp/base")
	if err != nil {
		log.Fatal(err)
	}

	for i := 1; i <= patch_num; i++ {
		f, err := os.Open("restored/.tmp/base")
		if err != nil {
			log.Fatal(err)
		}
		m, err := os.OpenFile("restored/.tmp/master", os.O_CREATE|os.O_WRONLY, 0777)
		if err != nil {
			log.Fatal(err)
		}
		p, err := os.Open("restored/.tmp/" + c[i].Name)
		if err != nil {
			log.Fatal(err)
		}
		err = binarydist.Patch(f, m, p)
		if err != nil {
			log.Fatal(err)
		}
		m.Close()
		f.Close()
		p.Close()

		os.Rename("restored/.tmp/master", "restored/.tmp/base")
	}
	return "restored/.tmp/base"
}

// Store the pid so we can kill the process later.

func store_pid() {
	pid := os.Getpid()
	f, err := os.OpenFile(".smog/.pid", os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	f.Write([]byte(strconv.Itoa(pid)))
}

// Kill the watcher processes

func kill_smog() {
	f, err := ioutil.ReadFile(".smog/.pid")
	if err != nil {
		log.Fatal(err)
	}
	pid := string(f)
	c := exec.Command("kill", "-2", pid)
	c.Run()
	os.Remove(".smog/.pid")
}

func watch() {
	store_pid()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	ev := make(chan *fsnotify.FileEvent)

	s := initialize()
	db := load_db()
	q := init_queue(s.URI, db)
	fmt.Printf("%v\n", s.URI)

	// Initialize event watcher
	go func() {
		for {
			select {
			case event := <-watcher.Event:
				ev <- event
			case err := <-watcher.Error:
				log.Println("Watcher error:", err)
			}
		}
	}()

	err = watcher.Watch("./")
	if err != nil {
		log.Fatal(err)
	}

	for {
		e := <-ev

		// Check to see if the action was on a file or a directory.
		// If it was on a directory, skip it.
		if !e.IsDelete() {
			f, err := os.Open(e.Name)
			if err != nil {
				log.Printf("%v\n", err)
				continue
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				log.Printf("%v\n", err)
				continue
			}
			if mode := fi.Mode(); mode.IsDir() {
				continue
			}
		}

		// Check for a list of file types we don't want to upload
		if strings.HasPrefix(e.Name, ".DS") {
			continue
		} else if strings.HasPrefix(e.Name, "#") {
			continue
		} else if strings.HasSuffix(e.Name, "~") {
			continue
		} else if strings.HasSuffix(e.Name, ".go") {
			continue
		}

		if e.IsCreate() {
			q.add(e.Name, "NEW")
		} else if e.IsDelete() {
			q.add(e.Name, "DELETE")
		} else if e.IsRename() {
			q.add(e.Name, "DELETE")
		} else if e.IsModify() {
			q.add(e.Name, "UPDATE")
		}
	}
}

func print_help() {
	fmt.Printf("\n")
	fmt.Printf("Usage: smog <command> <command-options>\n\n")
	fmt.Printf("Commands:\n")
	fmt.Printf("\tstart\t\tStart folder monitoring.\n")
	fmt.Printf("\tstop\t\tStop folder monitoring.\n")
	fmt.Printf("\twatch\t\tRun the background daemon in the foreground.\n")
	fmt.Printf("\trestore\t\tRestore a previous version of the file.\n")
	fmt.Printf("\trenew\t\tRenew the leases on your files.\n")
	fmt.Printf("\n")
}

func print_restore_help() {
	fmt.Printf("\nUsage: smog restore [file]\n\n")
}

func main() {
	argc := len(os.Args)
	if argc < 2 {
		print_help()
		os.Exit(0)
	}
	command := os.Args[1]
	if command == "watch" {
		watch()
	} else if command == "start" {
		// Since Go doesn't have fork (and it looks like the core dev
		// doesn't want to implement it any time soon), here is
		// a hacky solution to start the watcher in the background.
		c := exec.Command("./smog", "watch", "&")
		c.Start()
		os.Exit(0)
	} else if command == "stop" {
		kill_smog()
	} else if command == "restore" {
		if argc < 3 {
			print_restore_help()
			os.Exit(0)
		}
		db := load_db()
		if !restore_file(os.Args[2], db) {
			fmt.Printf("smog: Cannot find file: %s\n", os.Args[2])
		}
		os.Exit(0)
	} else if command == "renew" {
		renew_all()
	} else {
		print_help()
	}
}
