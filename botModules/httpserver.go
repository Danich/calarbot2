package botModules

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func ServeModule(module BotModule, addr string) error {
	http.HandleFunc("/is_called", func(w http.ResponseWriter, r *http.Request) {
		var payload Payload
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			fmt.Printf("error decoding payload: %w", err)
		}
		result := module.IsCalled(payload.Msg)
		err = json.NewEncoder(w).Encode(map[string]bool{"called": result})
		if err != nil {
			fmt.Printf("error encoding response: %w", err)
		}
	})

	http.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request) {
		var msg Payload
		err := json.NewDecoder(r.Body).Decode(&msg)
		if err != nil {
			log.Println(err)
		}
		answer, err := module.Answer(&msg)
		resp := map[string]interface{}{"answer": answer}
		if err != nil {
			resp["error"] = err.Error()
		}
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			resp["error"] = err.Error()
		}
	})

	return http.ListenAndServe(addr, nil)
}
