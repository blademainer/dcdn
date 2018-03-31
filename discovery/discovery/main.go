package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	postgis "github.com/cridenour/go-postgis"
	_ "github.com/lib/pq"
)

//FreeGeoIP json response format
type geojson struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	RegionCode  string  `json:"region_code"`
	RegionName  string  `json:"region_name"`
	City        string  `json:"city"`
	ZipCode     string  `json:"zip_code"`
	TimeZone    string  `json:"time_zone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	MetroCode   uint    `json:"metro_code"`
}

func main() {
	//load flags
	var dbaddr string
	var h string
	var geo string
	var checker string
	var sqcf string
	flag.StringVar(&geo, "geo", "https://freegeoip.com/json/", "geoip server to use")
	flag.StringVar(&dbaddr, "db", "", "postgresql database to use")
	flag.StringVar(&h, "http", ":8080", "http bind address")
	flag.StringVar(&checker, "checker", "", "URL of checker microservice /check")
	flag.StringVar(&sqcf, "sqlcmd", "gentbl.sql", "file containing sql command to run on startup")
	flag.Parse()
	if dbaddr == "" {
		log.Fatalln("Missing database address")
	}
	if checker == "" {
		log.Fatalln("Missing checker address")
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
	//handle search for caches
	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		//find IP
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse IP: %q", err.Error()), http.StatusInternalServerError)
			return
		}
		//handle X-Real-IP
		if realip := r.Header.Get("X-Real-IP"); realip != "" {
			ip = realip
		}
		//check IP
		if net.ParseIP(ip) == nil {
			http.Error(w, fmt.Sprintf("Invalid IP: %q", ip), http.StatusInternalServerError)
			return
		}
		//geolocate ip
		g, err := http.Get(geo + ip)
		if err != nil {
			http.Error(w, "Geolocation failure", http.StatusFailedDependency)
			log.Printf("Geolocation failure: %q\n", err.Error())
			return
		}
		defer g.Body.Close()
		var loc geojson
		err = json.NewDecoder(g.Body).Decode(&loc)
		if err != nil {
			http.Error(w, "Geolocation failure", http.StatusFailedDependency)
			log.Printf("Geolocation json decode failure: %q\n", err.Error())
			return
		}
		//run database query
		q, err := db.Query("SELECT url FROM servers ORDER pts <-> st_setsrid($1,4326) LIMIT 10;", postgis.PointS{SRID: 4326, X: loc.Latitude, Y: loc.Longitude})
		if err != nil {
			http.Error(w, "database failure", http.StatusFailedDependency)
			log.Printf("database failure: %q\n", err.Error())
			return
		}
		defer q.Close()
		je := json.NewEncoder(w)
		var srvs [10]string
		i := 0
		for q.Next() {
			err = q.Scan(srvs[i])
			if err != nil {
				http.Error(w, "database failure", http.StatusFailedDependency)
				log.Printf("database scan error: %q\n", err.Error())
				return
			}
			i++
		}
		//send response
		err = je.Encode(srvs[:i])
		if err != nil {
			log.Printf("JSON encoding error: %q\n", err.Error())
			return
		}
	})
	//handle registration
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		//parse form
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		server := r.FormValue("url")
		if server == "" {
			http.Error(w, "missing url", http.StatusBadRequest)
			return
		}

		var resp struct {
			Ok     bool
			ErrMsg string
		}
		//parse URL
		surl, err := url.Parse(server)
		if err != nil {
			resp.ErrMsg = err.Error()
			goto sendresp
		}
		//check that the server is valid
		{
			p, err := http.PostForm(
				checker,
				url.Values{
					"targ": []string{
						surl.String(),
					},
				},
			)
			if err != nil {
				resp.ErrMsg = "backend error"
				log.Printf("Failed to contact checker: %q\n", err.Error())
				goto sendresp
			}
			var chkresp struct {
				Valid  bool   `json:"valid"`
				ErrMsg string `json:"error"`
			}
			err = json.NewDecoder(p.Body).Decode(&chkresp)
			if err != nil {
				resp.ErrMsg = "backend error"
				log.Printf("Failed to decode checker response: %q\n", err.Error())
				goto sendresp
			}
			//geolocate ip
			g, err := http.Get(geo + surl.Hostname())
			if err != nil {
				resp.ErrMsg = "geolocation failure"
				log.Printf("Geolocation failure: %q\n", err.Error())
				goto sendresp
			}
			var loc geojson
			defer g.Body.Close()
			err = json.NewDecoder(g.Body).Decode(&loc)
			if err != nil {
				resp.ErrMsg = "geolocation failure"
				log.Printf("Geolocation json decode failure: %q\n", err.Error())
				goto sendresp
			}
			//register with database
			_, err = db.Exec(
				`INSERT INTO servers (loc, url, https, lastUpdate)
				VALUES ($1, $2, $3, $4)
				ON conflict(url) DO UPDATE
					SET
						loc = excluded.loc,
						url = excluded.url,
						https = excluded.https,
						lastUpdate = excluded.lastUpdate;`,
				postgis.PointS{SRID: 4326, X: loc.Latitude, Y: loc.Longitude},
				surl.String(),
				surl.Scheme == "https",
				time.Now().Unix(),
			)
			if err != nil {
				resp.ErrMsg = "database error"
				log.Printf("database error: %q\n", err.Error())
				goto sendresp
			}
		}
		resp.Ok = true //if we got here then it must have worked
	sendresp:
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			http.Error(w, "missing url", http.StatusInternalServerError)
			return
		}
	})
}
