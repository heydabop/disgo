package climacell

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"errors"
)

type aqiResponse struct {
	Data struct {
		Timelines []struct {
			Timestep string `json:"timestep"`
			Intervals []struct {
				Values struct {
					EpaIndex uint64 `json:"epaIndex"`
				} `json:"values"`
			} `json:"intervals"`
		} `json:"timelines"`
	} `json:"data"`
}

//GetAQI returns the current EPA AQI for lat,lng
func GetAQI(lat, lng float64, apiKey string) (uint64, error) {
	res, err := http.Get(fmt.Sprintf("https://api.tomorrow.io/v4/timelines?location=%f,%f&timesteps=current&fields=epaIndex&apikey=%s", lat, lng, apiKey))
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
	if len(response.Data.Timelines) == 1 && len(response.Data.Timelines[0].Intervals) == 1 && response.Data.Timelines[0].Timestep == "current" {
		return response.Data.Timelines[0].Intervals[0].Values.EpaIndex, nil
	}
	return 0, errors.New("response missing AQI")
}
