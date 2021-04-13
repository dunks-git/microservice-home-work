## Microservice in Go language, which reads exchange rates from [https://www.bank.lv/vk/ecb_rss.xm](https://www.bank.lv/vk/ecb_rss.xml) RSS feed and displays it to the user.
---
### **Dependencies (should be already installed):**
1. [cassandra db](https://cassandra.apache.org/)
2. [golang](https://golang.org/)
3. [curl](https://curl.se/)
---
### **Installation:**
1. Navigate to your %GOPATH% directory (like this:`cd %GOPATH%`),for example `C:\Users\user\go`

2. Create there if not exists directories:
    * `src`
    * `pkg`
    * `bin`

3. Open `src` directory
4. Copy whole downloded directory `lb_currency_rates` into `src` directory
5. Open that directory `lb_currency_rates`
6. Edit `main.go` file `const` section for Your needs according to comments in that section.
7. Edit for Your needs `main.go` file line `cluster := gocql.NewCluster("127.0.0.1")` in `func initSession()` according to comments above that line
8. Save file `main.go`
9. If You need, You can modify `go.mod` file and change line `module wwww` to `module executable_file_name_you_like_to_start_this_microservice`
10. If not yet started, start cassandra db server. (It can be done by runing cassandra executable from `cassandra/bin` directory)
11. Create keyspace and table in cassandra (You can use `cqlsh` from cassandra `bin` directory) (If You have changed `KEYSPACE` in `const` section of file `main.go` change `cur_rates` keyspace to yours). Example to run from `cqlsh`:
    * `create KEYSPACE IF NOT EXISTS cur_rates(or your keyspace) with replication = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 };`
    * `use cur_rates;` (or your keyspace)
    * `create table IF NOT EXISTS euro_rates(pub_date timestamp, rates text, PRIMARY KEY(pub_date));`
    * `create index on euro_rates(rates);`
12. Navigate in console to `lb_currency_rates` and run the command:
`go install`
13. Check if new executable `wwww`(or `executable_file_name_you_like_to_start_this_microservice`) appears in the `%GOPATH%/bin` directory
14. Navigate in console to `%GOPATH%/bin` directory and run `wwww`(or `executable_file_name_you_like_to_start_this_microservice`)
15. Load data to newly created table `euro_rates` by running command that sends request to this microservice endpoint `/currencies/set/` using `PUT` method and `HTTP_AUTH` header for authentication (if you have changed `HTTP_AUTH` and/or `LISTEN_AND_SERVE_PORT` in `const` section of `main.go` change this command too)
*  `/currencies/set/`:
`curl -v -H "HTTP_AUTH:QmFzaWMgeHh4" -X PUT http://localhost:8084/currencies/set/`

* (Note, that endpoint should ends with slash)
* If one of the result lines contains `201 Created`, data is loaded into the table `euro_rates`
16. Test microservice end-points in browser. These end-points accept only `GET` method and do not check `HTTP_AUTH` header.  (change port in these requests if you have changed `LISTEN_AND_SERVE_PORT` in `const` section of `main.go` ) :
    * for Russian roubles in date descending order:
    * `http://localhost:8084/currencies/one/rub/desc/`
    * for GB Pounds in date ascending order:
    * `http://localhost:8084/currencies/one/gbp/asc/`
    * for all latest:
    * `http://localhost:8084/currencies/latest/`

17. End-points return data in json format. Examples of returned data:
    * for one currency `rub`:
    * `[{"date":"2021-04-09T00:00:00Z","rate":91.8152},{"date":"2021-04-08T00:00:00Z","rate":91.4618},{"date":"2021-04-07T00:00:00Z","rate":92.3359}]`
    * for all currencies latest:
    ```{"date":"2021-04-09 00:00:00 +0000 UTC","rates":{"AUD":1.55790000,"BGN":1.95580000,"BRL":6.66410000,"CAD":1.49500000,"CHF":1.10100000,"CNY":7.79340000,"CZK":25.94500000,"DKK":7.43720000,"GBP":0.86658000,"HKD":9.24700000,"HRK":7.57550000,"HUF":357.97000000,"IDR":17354.52000000,"ILS":3.90930000,"INR":88.81450000,"ISK":151.90000000,"JPY":130.42000000,"KRW":1331.28000000,"MXN":23.93740000,"MYR":4.91570000,"NOK":10.11300000,"NZD":1.68600000,"PHP":57.76400000,"PLN":4.53920000,"RON":4.91980000,"RUB":91.81520000,"SEK":10.17250000,"SGD":1.59410000,"THB":37.38800000,"TRY":9.69030000,"USD":1.18880000,"ZAR":17.31000000}}```
18. Watch 11 minutes video in latvian as proof that this code works:
    * https://youtu.be/27Nw4IHb30c


