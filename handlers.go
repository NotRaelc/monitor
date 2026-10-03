package main

import (
	"encoding/json"
	"fmt"
	"goPrac/query"
	"net/http"
	"strconv"
)

func handleServerInput(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	tempMode := r.URL.Query().Get("mode")
	mode := 0
	switch tempMode {
	case "1", "2", "3":
		mode, _ = strconv.Atoi(tempMode)
	}
	server, err := query.QueryServer(address, query.DefaultTimeout, query.ServerType(mode))
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err)
		return
	}
	data, _ := json.MarshalIndent(server, "", " ")
	fmt.Fprint(w, string(data))
	return
}
