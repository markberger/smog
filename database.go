package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
)

type file_info struct {
	Name       string
	Base_num   int
	Patch_num  int
	Linkmotime float64
	Renew      bool
	URI        string
}

type _db struct {
	files map[string]file_info
}

func create_db() *_db {
	f, err := os.OpenFile(".smog/db.json", os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	db := new(_db)
	m := make(map[string]file_info)
	db.files = m
	db.save()
	return db
}

func load_db() *_db {
	f, err := os.Open("./.smog/db.json")
	if err != nil {
		if os.IsNotExist(err) {
			return create_db()
		} else {
			log.Fatal(err)
		}
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	m := make(map[string]file_info)
	err = decoder.Decode(&m)
	if err != nil {
		log.Fatal(err)
	}

	db := new(_db)
	db.files = m
	return db
}

func (db *_db) add(name string, base_num int, patch_num int, motime float64, uri string) {
	var f file_info
	f.Name = name
	f.Base_num = base_num
	f.Patch_num = patch_num
	f.Linkmotime = motime
	f.Renew = true
	f.URI = uri
	db.files[name] = f
}

func (db *_db) get_renewables() []file_info {
	var to_renew []file_info
	for _, val := range db.files {
		if val.Renew {
			to_renew = append(to_renew, val)
		}
	}
	return to_renew
}

func (db *_db) get_file(name string) (file_info, error) {
	f, present := db.files[name]
	if !present {
		return f, errors.New("Does not exist.")
	} else {
		return f, nil
	}
}

func (db *_db) get_patchnum(name string) int {
	f := db.files[name]
	return f.Patch_num
}

func (db *_db) get_basenum(name string) int {
	f := db.files[name]
	return f.Base_num
}

func (db *_db) update_renew(name string, renew bool) {
	f := db.files[name]
	f.Renew = renew
	db.files[name] = f
}

func (db *_db) update_patchnum(name string, num int) {
	f := db.files[name]
	f.Patch_num = num
	db.files[name] = f
}

func (db *_db) update_linkmotime(name string, time float64) {
	f := db.files[name]
	f.Linkmotime = time
	db.files[name] = f
}

func (db *_db) save() {
	f, err := os.OpenFile(".smog/db-tmp.json", os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Fatal(err)
	}

	e := json.NewEncoder(f)
	err = e.Encode(db.files)
	if err != nil {
		f.Close()
		log.Fatal(err)
	}
	f.Close()
	os.Remove(".smog/db.json")
	err = os.Rename(".smog/db-tmp.json", ".smog/db.json")
	if err != nil {
		log.Fatal(err)
	}
}
