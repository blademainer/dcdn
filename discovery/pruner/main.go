package main

import (
	"database/sql"
	"flag"
	"io/ioutil"
	"log"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	//load flags
	var sqcf string
	var dbaddr string
	var refreshtime time.Duration
	var ttl time.Duration
	flag.StringVar(&sqcf, "sqlcmd", "gentbl.sql", "file containing sql command to run on startup")
	flag.StringVar(&dbaddr, "db", "", "postgresql database to use")
	flag.DurationVar(&refreshtime, "refresh", time.Minute, "time between scans")
	flag.DurationVar(&ttl, "ttl", 20*time.Minute, "time until flushing server from DB")
	flag.Parse()
	if dbaddr == "" {
		log.Fatalln("Missing database address")
	}
	//connect to db
	db, err := sql.Open("postgres", dbaddr)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %q\n", err.Error())
	}
	//run startup command (gentbl.sql usually)
	sqcmd, err := ioutil.ReadFile(sqcf)
	if err != nil {
		log.Fatalf("Failed to load command file %q: %q\n", sqcf, err.Error())
	}
	_, err = db.Exec(string(sqcmd))
	if err != nil {
		log.Fatalf("Failed to execute SQL startup command: %q\n", err.Error())
	}
	ref := time.NewTicker(refreshtime)
	for t := range ref.C {
		log.Println("Starting prune. . . ")
		ut := t.Unix() - int64(ttl.Seconds())
		res, err := db.Exec("DELETE FROM servers WHERE lastUpdate < $1;", ut)
		if err != nil {
			log.Fatalf("Failed to prune: %q\n", err.Error())
		}
		n, err := res.RowsAffected()
		if err != nil {
			log.Printf("Pruned %d entries\n", n)
		} else {
			log.Printf("Failed to get number of affected entries: %q\n", err.Error())
		}
	}
}
