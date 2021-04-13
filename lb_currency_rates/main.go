package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/gorilla/mux"
)

const (
	// HTTP_AUTH header security key for authorize
	// You can change this key for your needs
	HTTP_AUTH = "QmFzaWMgeHh4"

	// endpoint to check if service is up
	HOME_PAGE = "/"

	// endpoint to get latest currency rates
	CURRENCIES_LATEST = "/currencies/latest/"

	// endpoint to get latest currency rate for particular currency
	// example: for Australian dollar in descending order(latest dates first) http://localhost:8084/currencies/one/aud/desc/ (slash in the end is mandatory)
	CURRENCIES_ONE = "/currencies/one/{id:[a-zA-Z]{3}}/{sort:[ascde]{3,4}}/"

	// endpoint to set currency rates into db
	CURRENCIES_SET        = "/currencies/set/"
	LISTEN_AND_SERVE_PORT = ":8084"
	DEFAULT_CONTENT_TYPE  = "application/json; charset=utf-8"

	// URI to get currency rates from The Bank of Latvia
	XML_CURRENENCIES_URI = "https://www.bank.lv/vk/ecb_rss.xml"

	// IF USE_CLUSTER_AUTH set to true, replace the CASSANDRA_USERNAME and CASSANDRA_PASSWORD fields with their real settings.
	// Note that this user must have permissions to edit keyspace and tables
	USE_CLUSTER_AUTH   = false
	CASSANDRA_USERNAME = "cassandra"
	CASSANDRA_PASSWORD = "cassandra"

	// IF You need to use another keyspace, replace KEYSPACE field value with your keyspace
	KEYSPACE = "cur_rates"
)

var Session *gocql.Session

func getXML(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("status error: %v", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("read body: %v", err)
	}

	return data, nil
}

type CurrenciesRSS struct {
	XMLName  xml.Name        `xml:"rss" json:"rss"`
	Channels []CurrenciesXML `xml:"channel" json:"channels"`
}

type CurrenciesXML struct {
	XMLName xml.Name `xml:"channel" json:"channel"`
	Items   []Item   `xml:"item" json:"items"`
}

type Item struct {
	// XMLName     xml.Name `xml:"item" json:"item"`
	// Title       string   `xml:"title" json:"title"`
	// Link        string   `xml:"link" json:"link"`
	// Guid        string   `xml:"guid" json:"guid"`
	Description string `xml:"description" json:"description"`
	PubDate     string `xml:"pubDate" json:"pubDate"`
}

type Rate struct {
	PubDate time.Time `json:"date"`
	Rate    float32   `json:"rate"`
}

// get exchange rates from https://www.bank.lv/vk/ecb_rss.xml and put into db table euro_rates
// if all go well returns status code 201 Created
func setRates(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", DEFAULT_CONTENT_TYPE)

	if r.Header.Get("HTTP_AUTH") != HTTP_AUTH {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	xmlBytes, err := getXML(XML_CURRENENCIES_URI)
	if err != nil {
		w.WriteHeader(http.StatusFailedDependency)
		fmt.Fprintf(w, "{\"error\":\"failed to get XML: %v\"}", err)
		log.Printf("Failed to get XML: %v", err)
		return
	}

	currencies := &CurrenciesRSS{}
	err = xml.Unmarshal(xmlBytes, currencies)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprintf(w, "{\"error\":\"%v\"}", err)
		log.Printf("Failed to get XML: %v", err)
		return
	}

	lenItems := len(currencies.Channels[0].Items)

	for i := 0; i < lenItems; i++ {

		itemptr := &currencies.Channels[0].Items[i]

		tsAfter, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", itemptr.PubDate)
		if nil != err {
			w.WriteHeader(http.StatusFailedDependency)
			fmt.Fprintf(w, "{\"error\":\"%v\"}", err)
			log.Printf("{\"error\":\"%v\"}", err)
			return
		}

		var old_date time.Time
		if err := Session.Query(`SELECT pub_date FROM euro_rates WHERE pub_date = ? LIMIT 1`,
			tsAfter).Consistency(gocql.One).Scan(&old_date); err != nil && err != gocql.ErrNotFound {
			w.WriteHeader(http.StatusNoContent)
			fmt.Fprintf(w, "{\"error\":\"Error getting data from db: %v\"}", err)
			log.Printf("Error getting data from db: %v", err)
			return
		}
		if old_date.IsZero() {
			if err := Session.Query(`INSERT INTO euro_rates (pub_date, rates) VALUES (?, ?)`,
				tsAfter, itemptr.Description).Exec(); err != nil {
				w.WriteHeader(http.StatusNoContent)
				fmt.Fprintf(w, "{\"error\":\"Error getting data from db: %v\"}", err)
				log.Printf("Error getting data from db: %v", err)
				return
			}

			words := strings.Fields(itemptr.Description)
			len_words := len(words)

			for j := 0; j < len_words; j++ {

				new_col := strings.ToLower(words[j])
				col := ""
				if err := Session.Query("SELECT column_name FROM system_schema.columns WHERE keyspace_name = '"+KEYSPACE+"' AND table_name = 'euro_rates' and column_name = ? limit 1",
					new_col).Consistency(gocql.One).Scan(&col); err != nil && err != gocql.ErrNotFound {
					log.Print(err)
					return
				}

				j++

				if col == "" {
					if err := Session.Query("ALTER TABLE euro_rates ADD ( " + new_col + " float)").Exec(); err != nil {
						log.Print(err)
						return
					}

				}

				if err := Session.Query("UPDATE euro_rates set "+new_col+" = "+words[j]+" where pub_date = ? ",
					tsAfter).Consistency(gocql.One).Exec(); err != nil {
					log.Print(err)
					return
				}

			}

		}
	}
	w.WriteHeader(http.StatusCreated)
}

// write latest currency exchange rates in json format to output (browser)
func currencyPageLatest(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", DEFAULT_CONTENT_TYPE)

	var max_date time.Time
	if err := Session.Query(`select Max(pub_date)as max_date from euro_rates limit 1`).Consistency(gocql.One).Scan(&max_date); err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"error\":\"not found: %v\"}", err)
		return
	}

	var rates string
	if err := Session.Query(`select rates from euro_rates where pub_date = ? limit 1`,
		max_date).Consistency(gocql.One).Scan(&rates); err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"error\":\"not found: %v\"}", err)
		return
	}
	words := strings.Fields(rates)

	len_words := len(words)

	json_rates := "{"
	json_rates += "\"date\":\"" + max_date.String() + "\",\"rates\":{"
	for i := 0; i < len_words; i++ {
		json_rates += "\"" + words[i] + "\":"
		i++
		json_rates += words[i]
		if i < len_words-1 {
			json_rates += ","
		} else {
			json_rates += "}"
		}
	}
	json_rates += "}"

	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, json_rates)

}

// write one currency exchange rates in json format to output (browser)
func currencyOne(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", DEFAULT_CONTENT_TYPE)
	vars := mux.Vars(r)

	if currency, ok := vars["id"]; !ok {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(w, "{\"error:\":\"%v not found\"}", currency)
		return
	}

	if srt, ok := vars["sort"]; !ok || (srt != "asc" && srt != "desc") {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(w, "{\"error:\":\"sort %v not found\"}", srt)
		return
	}

	col := ""
	if err := Session.Query("SELECT column_name FROM system_schema.columns WHERE keyspace_name = '"+KEYSPACE+"' AND table_name = 'euro_rates' and column_name = ? limit 1",
		vars["id"]).Consistency(gocql.One).Scan(&col); err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"error\":\"column %v not exists: %v\"}", vars["id"], err)
		return
	}

	var pub_date time.Time
	var rate float32

	iter := Session.Query("select pub_date, " + vars["id"] + " from euro_rates ").Iter()

	val_rates := []Rate{}
	vptr := &val_rates

	for iter.Scan(&pub_date, &rate) {

		new_rate := Rate{pub_date, rate}
		*vptr = append(*vptr, new_rate)
	}

	if err := iter.Close(); err != nil {
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprintf(w, "{\"error\":\"No content: %v\"}", err)
		return
	}

	if vars["sort"] == "desc" {
		sort.Slice(val_rates, func(i, j int) bool {
			return val_rates[i].PubDate.After(val_rates[j].PubDate)
		})
	} else if vars["sort"] == "asc" {
		sort.Slice(val_rates, func(i, j int) bool {
			return val_rates[i].PubDate.Before(val_rates[j].PubDate)
		})
	}

	b, err := json.Marshal(*vptr)
	if err != nil {

		w.WriteHeader(http.StatusNoContent)
		fmt.Fprintf(w, "{\"error\":\"No content: %v\"}", err)
		return
	}

	// Convert bytes to string.
	s := string(b)
	fmt.Fprint(w, s)

}
func homePage(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", DEFAULT_CONTENT_TYPE)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "{\"ping:\":\"OK\"}")

}

func handlRequest() {

	router := mux.NewRouter()

	router.HandleFunc(HOME_PAGE, homePage).Methods("GET")
	router.HandleFunc(CURRENCIES_LATEST, currencyPageLatest).Methods("GET")
	router.HandleFunc(CURRENCIES_ONE, currencyOne).Methods("GET")
	router.HandleFunc(CURRENCIES_SET, setRates).Methods("PUT")

	http.Handle(HOME_PAGE, router)
	//http.ListenAndServe(LISTEN_AND_SERVE_PORT, nil)
	log.Fatal(http.ListenAndServe(LISTEN_AND_SERVE_PORT, nil))

}

// init cassandra db session and set it to global Session
func initSession() {

	// cluster := gocql.NewCluster("PublicIP", "PublicIP", "PublicIP")
	// replace PublicIP with the IP addresses used by your cluster AND COMMENT or delete next line // cluster := gocql.NewCluster("127.0.0.1")
	cluster := gocql.NewCluster("127.0.0.1")

	cluster.Keyspace = KEYSPACE
	cluster.Consistency = gocql.Quorum
	// cluster.ProtoVersion = 4
	cluster.ConnectTimeout = time.Second * 10
	if USE_CLUSTER_AUTH {
		cluster.Authenticator = gocql.PasswordAuthenticator{Username: CASSANDRA_USERNAME, Password: CASSANDRA_PASSWORD}
	}
	session, err := cluster.CreateSession()
	if err != nil {
		//w.WriteHeader(http.StatusFailedDependency)
		//fmt.Fprintf(w, "{\"error\":\"Error creating the session: %v\"}", err)
		log.Printf("Error creating the session: %v", err)
		return
	}

	Session = session

}

func main() {

	initSession()
	defer Session.Close()

	handlRequest()

}
