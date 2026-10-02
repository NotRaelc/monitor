package main

import (
	"encoding/json"
	"fmt"
	"goPrac/query"
	"net/http"
)

func handleServerInput(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	server, err := query.QueryServer(address, query.DefaultTimeout)
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err)
		return
	}
	data, _ := json.MarshalIndent(server, "", " ")
	fmt.Fprint(w, string(data))
	return
}
