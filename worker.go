package main

import (
	"strings"
)

// send ip to ch for add it to ping list
func worker(ip string, ch cCustomer) {
	var (
		err error
		status,
		statusOld string
		// not paused on start: the pause only comes from an actual ⏸️
		// command, see workHours (…⏸️ at 18:00, …🔁 at 08:00 MSK)
		paused bool
		cus    = customers{}
	)
	defer wg.Done()
	defer ips.del(ip, false)
	defer hosts.del(ip)
	// unsubscribe a customer: delete its status reply, the same as the ❌
	// button on that reply
	unsub := func(cu customer) {
		if cu.ReplyID != 0 {
			if err := deleteReply(cu.PeerID, cu.ReplyID); err != nil {
				let.Println(err)
			}
		}
	}
	for {
		select {
		case <-mainCtx.Done():
			for _, cu := range cus {
				cu.Cmd = ip
				save <- cu
			}
			ltf.Println("done", ip)
			return
		case cust, ok := <-ch:
			if !ok {
				ltf.Println("channel closed", ip)
				return
			}
			if cust.Cmd == ip { //load
				if cust.MsgID != 0 {
					ok, err := requestExists(cust)
					if err != nil {
						let.Println("requestExists", cust, err)
					} else if !ok {
						ltf.Println("drop orphan", cust)
						unsub(cust)
						continue // request deleted, do not re-subscribe
					}
				}
				cus = append(cus, cust)
				ltf.Println("loaded ", ip, paused)
			} else if cust.MsgID == 0 { //update from buttons
				switch cust.Cmd {
				case "⏸️":
					paused = true
					if cust.ReplyID != 0 {
						// foreign pause: the pressed reply was replaced
						// with a protocol one by the presser; adopt it as
						// the author's status reply (cust.UserID)
						for i, cu := range cus {
							if cu.UserID == cust.UserID {
								cus[i].ReplyID = cust.ReplyID
							}
						}
					}
				case cmdVerify: // restart: re-verify that request messages still exist
					kept := cus[:0]
					for _, cu := range cus {
						if cu.MsgID == 0 {
							kept = append(kept, cu)
							continue
						}
						ok, err := requestExists(cu)
						if err != nil {
							let.Println("requestExists", cu, err) // API error - keep the subscriber
							kept = append(kept, cu)
						} else if !ok {
							ltf.Println("drop orphan", cu)
							unsub(cu)
						} else {
							kept = append(kept, cu)
						}
					}
					cus = kept
					if len(cus) == 0 {
						ltf.Println("no subscribers", ip)
						return // defer ips.del removes the ip from monitoring
					}
					hosts.set(ip, status, cus)
					continue
				case "❎": // reply hidden by its author: drop that subscriber only,
					// in every peer - the presser is the same person
					kept := cus[:0]
					for _, cu := range cus {
						if cu.UserID == cust.UserID {
							ltf.Println("unsubscribe", cu)
							unsub(cu) // remove their replies in other peers too
							continue
						}
						kept = append(kept, cu)
					}
					cus = kept
					if len(cus) == 0 {
						ltf.Println("no subscribers", ip)
						return // defer ips.del removes the ip from monitoring
					}
					hosts.set(ip, status, cus)
					continue
				case "🔁":
					paused = false
				default:
					if strings.HasSuffix(cust.Cmd, "❌") {
						tsX := strings.TrimSuffix(cust.Cmd, "❌") // empty|pause|connect|disconnect
						if tsX == "" || strings.HasSuffix(status, tsX) || strings.HasPrefix(status, tsX) || (strings.HasPrefix(status, "❗") && tsX == "❗") {
							for _, cu := range cus {
								ltf.Println("unsubscribe", cu)
								unsub(cu)
							}
							return
						}
					}
				}
			} else { //subscribe
				cus = append(cus, cust)
			}
			statusOld = status
			ltf.Println(ip, cust, len(ch), status, paused)
			if !paused {
				status, err = ping(ip)
				if err != nil {
					status = "❗"
					ltf.Println("ping", ip, err)
					//return
				}
			} else {
				if status == "" { // first cycle after start: seed the base
					// status once, even when already paused by afterHours
					status, err = ping(ip)
					if err != nil {
						status = "❗"
						ltf.Println("ping", ip, err)
					}
				}
				if !strings.HasSuffix(status, "⏸️") {
					status += "⏸️"
				}
			}
			for i, cu := range cus {
				if cu.PeerID == 0 { // guard against zero entries
					continue
				}
				ltf.Println(i, cu.PeerID, cu.UserID, cu.MsgID, ip, cu.ReplyID, status, statusOld)
				// foreign pause: the author's cu already points at the new
				// protocol reply adopted above, keep it as is
				if cust.Cmd == "⏸️" && cust.ReplyID != 0 && cu.ReplyID == cust.ReplyID {
					continue
				}
				if cu.ReplyID == 0 || status != statusOld {
					unsub(cu)
					cus[i].ReplyID, err = sendStatusReply(cu, status+" "+ip)
					if err != nil {
						letf.Println("send", ip, err)
						ips.del(ip, false)
					}
				}
			}
			hosts.set(ip, status, cus)
		}
	}
}
