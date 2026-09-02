package main

import "github.com/SevereCloud/vksdk/v3/api"

// group description markers, switched on full bot stop/start (see main.go)
const (
	descWorking = "Пингвин🏓 работает ▶️"
	descStopped = "Пингвин🏓 остановлен ⏹️"
)

var groupID int // group_id of the community, resolved lazily once

// read current group_id and description: for a community token groups.getById
// without group_ids returns the current community itself
func groupInfo() (id int, desc string) {
	if bot == nil {
		return
	}
	res, err := bot.GroupsGetByID(api.Params{
		"fields": []string{"description"},
	})
	if err != nil {
		letf.Println("groups.getById", err)
		return
	}
	if len(res.Groups) == 0 {
		return
	}
	id = res.Groups[0].ID
	desc = res.Groups[0].Description
	return
}

// best-effort: change the group description only when it differs from the
// current one, never fail the caller
func setGroupDescription(target string) {
	if bot == nil {
		return
	}
	id, cur := groupInfo()
	if id == 0 {
		letf.Println("setGroupDescription: unknown group_id")
		return
	}
	groupID = id
	if cur == target {
		ltf.Println("groups.edit skipped, unchanged", id, cur)
		return
	}
	if _, err := bot.GroupsEdit(api.Params{
		"group_id":    id,
		"description": target,
	}); err != nil {
		letf.Println("groups.edit", id, err)
		return
	}
	ltf.Println("groups.edit", id, cur, "->", target)
}
