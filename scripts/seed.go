package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
)

const referencesURL = "https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/resources/references.json.gz"

// Schema of the data inside this json ^
// [
//   { "vector": [0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416], "label": "legit" },
//   { "vector": [0.5796, 0.9167, 1.0, 0.0435, 0, 0.0056, 0.4394, 0.4598, 0.4, 1, 0, 1, 0.85, 0.0032], "label": "fraud" }
// ]

type Reference struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

func SeedDb() {

	resp, err := http.Get(referencesURL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// unzip stream
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		panic(err)
	}
	defer gzReader.Close()

	decoder := json.NewDecoder(gzReader)

	// read opening '['
	_, err = decoder.Token()
	if err != nil {
		panic(err)
	}

	count := 0

	// stream each object
	for decoder.More() {
		var reference Reference

		if err := decoder.Decode(&reference); err != nil {
			panic(err)
		}

		// process item immediately
		fmt.Println(reference.Vector, reference.Label)

		count++
	}

	fmt.Println("processed:", count)

}

func main() {
	SeedDb()
}
