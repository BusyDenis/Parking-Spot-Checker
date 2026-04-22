package main

import (
	"encoding/json"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
)

var funcMap = template.FuncMap{
	"json": func(v any) (template.JS, error) {
		b, err := json.Marshal(v)
		return template.JS(b), err
	},
}

var templates = template.Must(template.New("").Funcs(funcMap).ParseGlob("*.html"))

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

// ParkenDD occasionally reports Free > Total (impossible); clamp to Total.
func correctedFree(lot parkingLot) int {
	if lot.Free > lot.Total {
		return lot.Total
	}
	return lot.Free
}

func markFull(lot *parkingLot) {
	if lot.Free == 0 {
		lot.State = "full"
	}
}

func markBusy(lot *parkingLot) {
	if lot.Free < lot.Total/3 {
		lot.State = "busy"
	}
}

func handler(w http.ResponseWriter, r *http.Request) {

	radius := r.URL.Query().Get("radius")
	if radius == "" {
		radius = "750"
	}

	latitude := r.URL.Query().Get("latitude")
	longitude := r.URL.Query().Get("longitude")

	resp, err := http.Get("https://api.parkendd.de/Zuerich")

	if err != nil {
		log.Println(err)
		http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to fetch data", http.StatusBadGateway)
		return
	}

	var data parkingResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		log.Println(err)
		http.Error(w, "Parse error", http.StatusInternalServerError)
		return
	}

	data.Lots = append(data.Lots, testLots...)

	filteredLots := data.Lots

	radiusValue, radiusErr := strconv.ParseFloat(radius, 64)
	latitudeValue, latitudeErr := strconv.ParseFloat(latitude, 64)
	longitudeValue, longitudeErr := strconv.ParseFloat(longitude, 64)

	if radiusErr == nil && latitudeErr == nil && longitudeErr == nil {
		filteredLots = nil

		for _, lot := range data.Lots {
			distance := distanceInMeters(latitudeValue, longitudeValue, lot.Coords.Lat, lot.Coords.Lng)
			if distance <= radiusValue {
				filteredLots = append(filteredLots, lot)
			}
		}
	}

	for i, lot := range filteredLots {
		filteredLots[i].Free = correctedFree(lot)
		markBusy(&filteredLots[i])
		markFull(&filteredLots[i])
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
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
