package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// apiAgentList returns the claudio.agent.list() binding.
//
// Lua usage:
//
//	local agents = claudio.agent.list()
//	-- agents is an array of {id=string, name=string, status=string, team=string}
func (r *Runtime) apiAgentList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.teamRunnerMu.RLock()
		runner := r.teamRunner
		r.teamRunnerMu.RUnlock()

		tbl := L.NewTable()
		if runner == nil {
			L.Push(tbl)
			return 1
		}

		for i, state := range runner.AllStates() {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(state.Identity.AgentID))
			L.SetField(entry, "name", lua.LString(state.Identity.AgentName))
			L.SetField(entry, "status", lua.LString(string(state.GetStatus())))
			L.SetField(entry, "team", lua.LString(state.TeamName))
			tbl.RawSetInt(i+1, entry)
		}

		L.Push(tbl)
		return 1
	}
}

// apiAgentStatus returns the claudio.agent.status(id) binding.
//
// Lua usage:
//
//	local s = claudio.agent.status("researcher@my-team")
//	-- returns "working"|"complete"|"failed"|"idle"|"shutdown"|"waiting_for_input", or nil
func (r *Runtime) apiAgentStatus(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		id := L.CheckString(1)

		r.teamRunnerMu.RLock()
		runner := r.teamRunner
		r.teamRunnerMu.RUnlock()

		if runner == nil {
			L.Push(lua.LNil)
			return 1
		}

		state, ok := runner.GetState(id)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(string(state.GetStatus())))
		return 1
	}
}

// apiTeamsList returns the claudio.teams.list() binding.
//
// Lua usage:
//
//	local names = claudio.teams.list()
//	-- names is an array of team name strings
func (r *Runtime) apiTeamsList(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		r.teamManagerMu.RLock()
		mgr := r.teamManager
		r.teamManagerMu.RUnlock()

		tbl := L.NewTable()
		if mgr == nil {
			L.Push(tbl)
			return 1
		}

		for i, cfg := range mgr.ListTeams() {
			tbl.RawSetInt(i+1, lua.LString(cfg.Name))
		}

		L.Push(tbl)
		return 1
	}
}

// apiTeamsMembers returns the claudio.teams.members(team_name) binding.
//
// Lua usage:
//
//	local members = claudio.teams.members("my-team")
//	-- members is an array of {id=string, name=string, role=string, status=string}
func (r *Runtime) apiTeamsMembers(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		teamName := L.CheckString(1)

		r.teamManagerMu.RLock()
		mgr := r.teamManager
		r.teamManagerMu.RUnlock()

		tbl := L.NewTable()
		if mgr == nil {
			L.Push(tbl)
			return 1
		}

		for i, m := range mgr.AllMembers(teamName) {
			entry := L.NewTable()
			L.SetField(entry, "id", lua.LString(m.Identity.AgentID))
			L.SetField(entry, "name", lua.LString(m.Identity.AgentName))
			L.SetField(entry, "role", lua.LString(m.SubagentType))
			L.SetField(entry, "status", lua.LString(string(m.Status)))
			tbl.RawSetInt(i+1, entry)
		}

		L.Push(tbl)
		return 1
	}
}
