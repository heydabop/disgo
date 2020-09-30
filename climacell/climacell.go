package climacell

import (
	"encoding/json"
	"math"
	"fmt"
	"io/ioutil"
	"net/http"
	"errors"
)

type aqiResponse struct {
	EpaAqi struct {
		Value float64 `json:"value"`
	} `json:"epa_aqi"`
}

//GetAQI returns the current EPA AQI for lat,lng
func GetAQI(lat, lng float64, apiKey string) (uint64, error) {
	res, err := http.Get(fmt.Sprintf("https://api.climacell.co/v3/weather/realtime?lat=%f&lon=%f&unit_system=si&fields=epa_aqi&apikey=%s", lat, lng, apiKey))
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return 0, errors.New(res.Status)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}
	var response aqiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, err
	}
	return uint64(math.Round(response.EpaAqi.Value)), nil
}
