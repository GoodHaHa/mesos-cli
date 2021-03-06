package commands

import (
	"github.com/jawher/mow.cli"
	"github.com/vektorlab/mesos-cli/commands/local"
	"github.com/vektorlab/mesos-cli/config"
)

var Commands = []Command{
	&Agents{},
	&List{},
	&local.Local{},
	&Read{},
	&Run{},
	&Task{},
	&Tasks{},
	&Top{},
}

type Command interface {
	Name() string
	Desc() string
	Init(config.ProfileFn) func(*cli.Cmd)
}
