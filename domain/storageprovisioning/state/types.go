// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/domain/life"

type netNode struct {
	UUID string `db:"net_node_uuid"`
}

type netNodeUUIDVal struct {
	UUID string `db:"uuid"`
}

type filesystemID struct {
	ID string `db:"filesystem_id"`
}

type filesystemLife struct {
	ID   string    `db:"filesystem_id"`
	Life life.Life `db:"life_id"`
}

type filesystemLives []filesystemLife

func (l filesystemLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

type volumeID struct {
	ID string `db:"volume_id"`
}

type volumeLife struct {
	ID   string    `db:"volume_id"`
	Life life.Life `db:"life_id"`
}

type volumeLives []volumeLife

func (l volumeLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

type attachmentUUID struct {
	UUID string `db:"uuid"`
}

type attachmentLife struct {
	UUID string    `db:"uuid"`
	Life life.Life `db:"life_id"`
}

type attachmentLives []attachmentLife

func (l attachmentLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.UUID, v.Life) {
			return
		}
	}
}

type machineLife struct {
	LifeId int `db:"life_id"`
}

type machineUUIDVal struct {
	UUID string `db:"uuid"`
}
