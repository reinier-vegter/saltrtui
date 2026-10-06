// Package assignments defines normalized highstate top-file assignments.
package assignments

import (
	"context"
	"sort"
)

// Top maps a responding minion ID to the environments and SLS files selected
// for its highstate. A missing ID is a nonresponse, while an empty environment
// map is a valid state.show_top result.
type Top map[string]map[string][]string

type Gateway interface {
	ReadStateTop(context.Context, []string) (Top, error)
}

type Row struct {
	Environment string
	State       string
	Minions     []string
}

type NodeKind uint8

const (
	EnvironmentNode NodeKind = iota
	StateNode
	MinionNode
)

// Node is one stable entry in the environment → SLS → minion assignment tree.
// ParentID is empty only for environment roots.
type Node struct {
	ID, ParentID, Environment, State, Minion string
	Kind                                     NodeKind
}

func EnvironmentID(environment string) string { return "environment\x00" + environment }
func StateID(environment, state string) string {
	return "state\x00" + environment + "\x00" + state
}
func MinionID(environment, state, minion string) string {
	return "minion\x00" + environment + "\x00" + state + "\x00" + minion
}

// Tree returns the complete stable hierarchy. Callers own expansion and
// filtering; no order depends on a Go map iteration.
func Tree(top Top) []Node {
	rows := Rows(top)
	byEnvironment := make(map[string][]Row)
	for _, row := range rows {
		byEnvironment[row.Environment] = append(byEnvironment[row.Environment], row)
	}
	environments := make([]string, 0, len(byEnvironment))
	for environment := range byEnvironment {
		environments = append(environments, environment)
	}
	sort.Strings(environments)
	var nodes []Node
	for _, environment := range environments {
		environmentID := EnvironmentID(environment)
		nodes = append(nodes, Node{ID: environmentID, Environment: environment, Kind: EnvironmentNode})
		for _, row := range byEnvironment[environment] {
			stateID := StateID(environment, row.State)
			nodes = append(nodes, Node{ID: stateID, ParentID: environmentID, Environment: environment, State: row.State, Kind: StateNode})
			for _, minion := range row.Minions {
				nodes = append(nodes, Node{ID: MinionID(environment, row.State, minion), ParentID: stateID, Environment: environment, State: row.State, Minion: minion, Kind: MinionNode})
			}
		}
	}
	return nodes
}

// Rows groups each returned environment/SLS pair by its responding minions.
func Rows(top Top) []Row {
	groups := make(map[string]*Row)
	for id, environments := range top {
		for environment, states := range environments {
			for _, state := range states {
				key := environment + "\x00" + state
				row := groups[key]
				if row == nil {
					row = &Row{Environment: environment, State: state}
					groups[key] = row
				}
				row.Minions = append(row.Minions, id)
			}
		}
	}
	rows := make([]Row, 0, len(groups))
	for _, row := range groups {
		sort.Strings(row.Minions)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Environment == rows[j].Environment {
			return rows[i].State < rows[j].State
		}
		return rows[i].Environment < rows[j].Environment
	})
	return rows
}
