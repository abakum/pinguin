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
				status = cust.Status
				deadline = time.Unix(cust.Deadline, 0)
				cus = append(cus, cust)
				ltf.Println("loaded ", ip, status, deadline)
			} else if cust.MsgID == 0 { //update
				switch cust.Cmd {
				case "⏸️":
					deadline = time.Now().Add(-refresh)
				case "🔁":
					deadline = time.Now().Add(dd)
				default:
					if strings.HasSuffix(cust.Cmd, "❌") {
						tsX := strings.TrimSuffix(cust.Cmd, "❌") // empty|pause|connect|disconnect
						if tsX == "" || strings.HasSuffix(status, tsX) || strings.HasPrefix(status, tsX) || (strings.HasPrefix(status, "❗") && tsX == "❗") {
							for _, cu := range cus {
								ltf.Println("deleteMessage", cu)
								if cu.ReplyID != 0 {
									err = deleteMessage(cu.PeerID, cu.ReplyID)
									if err != nil {
										let.Println(err)
									}
								}
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
					if cu.ReplyID != 0 {
						err = deleteMessage(cu.PeerID, cu.ReplyID)
						if err != nil {
							let.Println(err)
						}
					}
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
