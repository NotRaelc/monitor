package main

import (
	"fmt"
	"net/http"
)

type Handler struct {
	apiVersion int
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc(fmt.Sprintf("GET /api/v%d/{address}", h.apiVersion), handleServerInput)
}

func main() {
	mux := http.NewServeMux()
	handler := Handler{apiVersion: 1}

	handler.Routes(mux)

	http.ListenAndServe(":8080", mux)
}
