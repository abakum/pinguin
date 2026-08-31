package main

import (
	"strings"
	"time"
)

// send ip to ch for add it to ping list
func worker(ip string, ch cCustomer) {
	var (
		err error
		status,
		statusOld string
		deadline = time.Now().Add(dd)
		cus      = customers{}
	)
	defer wg.Done()
	defer ips.del(ip, false)
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
			for i, cu := range cus {
				cu.Cmd = ip
				if i == 0 {
					cu.Status = status
					cu.Deadline = deadline.Unix()
					ltf.Println("saved ", ip, status, deadline)
				}
				save <- cu
			}
			ltf.Println("done", ip)
			return
		case cust, ok := <-ch:
			if !ok {
				ltf.Println("channel closed", ip)
				return
			}
			if cust.Cmd == ip && cust.Deadline > 0 { //load
				if cust.MsgID != 0 || cust.GlobalID != 0 {
					ok, err := requestExists(cust)
					if err != nil {
						let.Println("requestExists", cust, err)
					} else if !ok {
						ltf.Println("drop orphan", cust)
						unsub(cust)
						continue // request deleted, do not re-subscribe
					}
				}
				status = cust.Status
				deadline = time.Unix(cust.Deadline, 0)
				cus = append(cus, cust)
				ltf.Println("loaded ", ip, status, deadline)
			} else if cust.MsgID == 0 && cust.GlobalID == 0 { //update from buttons
				switch cust.Cmd {
				case "⏸️":
					deadline = time.Now().Add(-refresh)
				case cmdVerify: // restart: re-verify that request messages still exist
					kept := cus[:0]
					for _, cu := range cus {
						if cu.MsgID == 0 && cu.GlobalID == 0 {
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
					continue
				case "🔁":
					deadline = time.Now().Add(dd)
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
			ltf.Println(ip, cust, len(ch), status, time.Now().Before(deadline))
			if time.Now().Before(deadline) {
				status, err = ping(ip)
				if err != nil {
					status = "❗"
					ltf.Println("ping", ip, err)
					//return
				}
			} else {
				if !strings.HasSuffix(status, "⏸️") {
					status += "⏸️"
				}
			}
			for i, cu := range cus {
				if cu.PeerID == 0 { // guard against zero entries
					continue
				}
				ltf.Println(i, cu.PeerID, cu.UserID, cu.MsgID, ip, cu.ReplyID, status, statusOld)
				if cu.ReplyID == 0 || status != statusOld {
					unsub(cu)
					cus[i].ReplyID, err = sendStatusReply(cu, status+" "+ip)
					if err != nil {
						letf.Println("send", ip, err)
						ips.del(ip, false)
					}
				}
			}
		}
	}
}
