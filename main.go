package main

import (
	"encoding/json"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var funcMap = template.FuncMap{
	"json": func(v any) (template.JS, error) {
		b, err := json.Marshal(v)
		return template.JS(b), err
	},
}

const API_URL = "https://api.parkendd.de/Zuerich"
const cacheFilePath = "response/response.json"
const cacheRefreshInterval = time.Minute

var templates = template.Must(template.New("").Funcs(funcMap).ParseGlob("*.html"))

var cacheMu sync.RWMutex
var cachedData parkingResponse

type parkingLot struct {
	Address string `json:"address"`
	Coords  coords `json:"coords"`
	Free    int    `json:"free"`
	Total   int    `json:"total"`
	Name    string `json:"name"`
	State   string `json:"state"`
}

type coords struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type parkingResponse struct {
	LastDownloaded string       `json:"last_downloaded"`
	LastUpdated    string       `json:"last_updated"`
	Lots           []parkingLot `json:"lots"`
}

type pageData struct {
	Radius         string
	LastDownloaded string
	LastUpdated    string
	Lots           []parkingLot `json:"lots"`
}

var testLots = []parkingLot{
	{
		Name:    "Test Lot Overflowed",
		Address: "Bahnhofstrasse 1, Oberglatt",
		Coords:  coords{Lat: 47.4643, Lng: 8.5232},
		Free:    62,
		Total:   60,
		State:   "open",
	},
	{
		Name:    "Test Lot Busy",
		Address: "Bahnhofstrasse 3, Oberglatt",
		Coords:  coords{Lat: 47.4644, Lng: 8.5234},
		Free:    10,
		Total:   50,
		State:   "open",
	},
	{
		Name:    "Test Lot Full",
		Address: "Bahnhofstrasse 5, Oberglatt",
		Coords:  coords{Lat: 47.4645, Lng: 8.5232},
		Free:    0,
		Total:   50,
		State:   "open",
	},
	{
		Name:    "Test Lot Normal",
		Address: "Bahnhofstrasse 7, Oberglatt",
		Coords:  coords{Lat: 47.4644, Lng: 8.5230},
		Free:    40,
		Total:   50,
		State:   "open",
	},
}

func hasCachedData(data parkingResponse) bool {
	return len(data.Lots) > 0 || data.LastDownloaded != "" || data.LastUpdated != ""
}

func saveCacheToFile(data parkingResponse) error {
	if err := os.MkdirAll(filepath.Dir(cacheFilePath), 0755); err != nil {
		return err
	}

	file, err := os.Create(cacheFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func loadCacheFromFile() (parkingResponse, error) {
	file, err := os.Open(cacheFilePath)
	if err != nil {
		return parkingResponse{}, err
	}
	defer file.Close()

	var data parkingResponse
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return parkingResponse{}, err
	}
	return data, nil
}

func refreshCacheOnce() error {
	data, err := fetchParkingData()
	if err != nil {
		return err
	}

	cacheMu.Lock()
	cachedData = data
	cacheMu.Unlock()

	return saveCacheToFile(data)
}

func getParkingData() (parkingResponse, error) {
	cacheMu.RLock()
	data := cachedData
	cacheMu.RUnlock()

	if hasCachedData(data) {
		return data, nil
	}

	if err := refreshCacheOnce(); err != nil {
		return parkingResponse{}, err
	}

	cacheMu.RLock()
	data = cachedData
	cacheMu.RUnlock()
	return data, nil
}

func startCacheRefresher() {
	go func() {
		ticker := time.NewTicker(cacheRefreshInterval)
		defer ticker.Stop()

		for range ticker.C {
			if err := refreshCacheOnce(); err != nil {
				log.Printf("cache refresh failed: %v", err)
			}
		}
	}()
}

func initCache() {
	data, err := loadCacheFromFile()
	if err == nil {
		cacheMu.Lock()
		cachedData = data
		cacheMu.Unlock()
	} else if !os.IsNotExist(err) {
		log.Printf("cache file read failed: %v", err)
	}

	if err := refreshCacheOnce(); err != nil {
		log.Printf("initial cache refresh failed, using existing cache if present: %v", err)
	}

	startCacheRefresher()
}

func correctedFree(lot parkingLot) int {
	if lot.Free > lot.Total {
		return lot.Total
	}
	return lot.Free
}

func markFull(lot parkingLot) parkingLot {
	if lot.Free == 0 {
		lot.State = "full"
	}
	return lot
}

func markBusy(lot parkingLot) parkingLot {
	if lot.Free < lot.Total/3 {
		lot.State = "busy"
	}
	return lot
}

func httpHandler(r *http.Request) (radius, latitude, longitude string) {
	radius = r.URL.Query().Get("radius")
	if radius == "" {
		radius = "750"
	}

	latitude = r.URL.Query().Get("latitude")
	longitude = r.URL.Query().Get("longitude")
	return
}

func fetchParkingData() (parkingResponse, error) {
	var data parkingResponse
	resp, err := http.Get(API_URL)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return data, http.ErrHandlerTimeout
	}

	err = json.NewDecoder(resp.Body).Decode(&data)
	return data, err
}

func filterLots(lots []parkingLot, radius, latitude, longitude string) []parkingLot {
	radiusValue, radiusErr := strconv.ParseFloat(radius, 64)
	latitudeValue, latitudeErr := strconv.ParseFloat(latitude, 64)
	longitudeValue, longitudeErr := strconv.ParseFloat(longitude, 64)

	if radiusErr != nil || latitudeErr != nil || longitudeErr != nil {
		return lots
	}

	filteredLots := make([]parkingLot, 0, len(lots))
	for _, lot := range lots {
		distance := distanceInMeters(latitudeValue, longitudeValue, lot.Coords.Lat, lot.Coords.Lng)
		if distance <= radiusValue {
			filteredLots = append(filteredLots, lot)
		}
	}

	return filteredLots
}

func handler(w http.ResponseWriter, r *http.Request) {

	radius, latitude, longitude := httpHandler(r)

	data, err := getParkingData()
	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to fetch data", http.StatusBadGateway)
		return
	}

	// Copy lots first so request-level mutations never touch shared cache memory.
	lots := append([]parkingLot(nil), data.Lots...)
	lots = append(lots, testLots...)

	filteredLots := filterLots(lots, radius, latitude, longitude)

	for i, lot := range filteredLots {
		filteredLots[i].Free = correctedFree(lot)
		filteredLots[i] = markBusy(filteredLots[i])
		filteredLots[i] = markFull(filteredLots[i])
	}

	page := pageData{
		Radius:         radius,
		LastDownloaded: data.LastDownloaded,
		LastUpdated:    data.LastUpdated,
		Lots:           filteredLots,
	}

	err = templates.ExecuteTemplate(w, "index.html", page)
	if err != nil {
		log.Println(err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func distanceInMeters(lat1 float64, lng1 float64, lat2 float64, lng2 float64) float64 {
	const earthRadius = 6371000.0

	lat1Rad := lat1 * math.Pi / 180
	lng1Rad := lng1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lng2Rad := lng2 * math.Pi / 180

	latDiff := lat2Rad - lat1Rad
	lngDiff := lng2Rad - lng1Rad

	a := math.Sin(latDiff/2)*math.Sin(latDiff/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(lngDiff/2)*math.Sin(lngDiff/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func main() {
	initCache()

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
