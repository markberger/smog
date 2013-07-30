package main

import (
	"fmt"
	"time"
)

type queue struct {
	todo   map[string]bool
	dircap string
	db     *_db
}

func init_queue(dircap string, db *_db) queue {
	var q queue
	q.dircap = dircap
	q.db = db
	m := make(map[string]bool)
	q.todo = m
	return q
}

func (q queue) add(file string, action string) {
	if val := q.todo[file]; !val {
		q.todo[file] = true
		fmt.Printf("Blocking any more actions on %s\n", file)
		go func() {
			time.Sleep(time.Minute)
			if action == "NEW" {
				new_upload(q.dircap, file, q.db)
			} else if action == "UPDATE" {
				upload(q.dircap, file, q.db)
			} else if action == "DELETE" {
				delete(q.dircap, file, q.db)
			}
			q.todo[file] = false
			fmt.Printf("Now accepting actions on %s\n", file)
		}()
	} else {
		fmt.Printf("Received action %s but file %s is blocked.\n", action, file)
	}
}
