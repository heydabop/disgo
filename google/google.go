package google

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

//Point is a Lat/Lng pair
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type geocodeResponse struct {
	Results []struct {
		Geometry struct {
			Location *Point `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
	Status string `json:"status"`
}

//Geocode attemps to geocode address with Google's Geocoding API, returning the lat/lng of the first result
func Geocode(address, mapsKey string) (*Point, error) {
	res, err := http.Get(fmt.Sprintf("https://maps.googleapis.com/maps/api/geocode/json?&address=%s&key=%s", url.QueryEscape(address), mapsKey))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var geo geocodeResponse
	if err := json.Unmarshal(body, &geo); err != nil {
		return nil, err
	}
	if geo.Status != "OK" {
		return nil, fmt.Errorf("expected status \"OK\" got \"%s\"", geo.Status)
	}
	if len(geo.Results) < 1 {
		return nil, fmt.Errorf("0 location results")
	}
	if geo.Results[0].Geometry.Location == nil {
		return nil, fmt.Errorf("missing geometry in result")
	}
	return geo.Results[0].Geometry.Location, nil
}
