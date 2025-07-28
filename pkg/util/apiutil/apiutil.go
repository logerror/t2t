package apiutil

import (
	"encoding/json"
	"net/http"
)

func RespondJSON(w http.ResponseWriter, r *http.Request, status int, output any) {
	data, err := json.Marshal(output)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(status)
	w.Write(data)
}
