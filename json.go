package main

import (
	"encoding/json"
	"os"
)

type config struct {
	M *mss
	C *customers
}

// load json
func loader() error {
	cus := customers{}
	conf := config{&dic, &cus}
	bytes, err := os.ReadFile(tmbPingJson)
	if err != nil {
		return srcError(err)
	}
	err = json.Unmarshal(bytes, &conf)
	if err != nil {
		return srcError(err)
	}
	for _, cu := range cus {
		ltf.Println(cu)
		ips.write(cu.Cmd, cu)
	}
	return nil
}

// save json
func saver() {
	defer wg.Done()
	cus := customers{}
	conf := config{&dic, &cus}
	for {
		select {
		case <-saveDone:
			ltf.Println(cus)
			bytes, err := json.Marshal(conf)
			if err != nil {
				PrintOk("", err)
				return
			}
			err = os.WriteFile(tmbPingJson, bytes, 0644)
			PrintOk("saver", err)
			return
		case cu, ok := <-save:
			if ok {
				cus = append(cus, cu)
				// bytes, _ := json.Marshal(cu)
				// stdo.Printf("%s\n", bytes)
			} else {
				ltf.Println("saver channel closed")
				saveDone <- true
			}
		}
	}
}
