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
	cus := customers{}
	conf := config{&dic, &cus}
	for {
		select {
		case <-saveDone:
			// all workers are done, drain buffered save records then write file
			for {
				select {
				case cu := <-save:
					cus = append(cus, cu)
				default:
					ltf.Println(cus)
					bytes, err := json.Marshal(conf)
					if err != nil {
						PrintOk("", err)
						break
					}
					err = os.WriteFile(tmbPingJson, bytes, 0644)
					PrintOk("saver", err)
					saverDone <- true
					return
				}
			}
		case cu := <-save:
			cus = append(cus, cu)
		}
	}
}
