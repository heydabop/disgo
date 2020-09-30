package darksky

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"errors"
)

//Weather is datapoints of the current weather and a summary of the forecase
type Weather struct {
		Currently struct {
			Summary             string  `json:"summary"`
			Temperature         float64 `json:"temperature"`
			ApparentTemperature float64 `json:"apparentTemperature"`
			DewPoint            float64 `json:"dewPoint"`
			Humidity            float64 `json:"humidity"`
			WindSpeed           float64 `json:"windSpeed"`
			WindGust            float64 `json:"windGust"`
			WindBearing         int     `json:"windBearing"`
			UvIndex             int     `json:"uvIndex"`
		} `json:"currently"`
		Hourly struct {
			Summary string `json:"summary"`
		} `json:"hourly"`
	}

//GetCurrent returns the current weather at lat,lng and a summary of the forecast for the day
func GetCurrent(lat, lng float64, apiKey string) (*Weather, error) {
	res, err := http.Get(fmt.Sprintf("https://api.darksky.net/forecast/%s/%f,%f?exclude=minutely,daily,alerts,flags&units=us&lang=en", apiKey, lat, lng))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New(res.Status)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var response Weather
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
