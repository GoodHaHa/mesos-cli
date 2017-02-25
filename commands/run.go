package commands

import (
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/jawher/mow.cli"
	"github.com/mesos/mesos-go"
	"github.com/vektorlab/mesos-cli/config"
	"github.com/vektorlab/mesos-cli/runner"
	"os"
	"strconv"
	"strings"
)

type Run struct{}

func (_ Run) Name() string { return "run" }
func (_ Run) Desc() string { return "Run tasks on Mesos" }

func (r *Run) Init(profile Profile) func(*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		cmd.Spec = "[OPTIONS] [CMD]"
		var (
			command  = cmd.StringArg("CMD", "", "Command to run")
			user     = cmd.StringOpt("user", "root", "User to run as")
			shell    = cmd.BoolOpt("shell", false, "Run as a shell command")
			hostname = cmd.StringOpt("master", "", "Mesos master")
			path     = cmd.StringOpt("path", "", "Path to a JSON file containing Mesos TaskInfos")
			dump     = cmd.BoolOpt("json", false, "Dump the task to JSON instead of running it")
			docker   = cmd.BoolOpt("docker", false, "Run as a Docker container")
			image    = cmd.StringOpt("image", "", "Image to run")
			restart  = cmd.BoolOpt("restart", false, "Restart container on failure")

			// Docker-only options
			privileged = cmd.BoolOpt("privileged", false, "Run in privileged mode [docker only]")
		)
		envs := &Envs{}
		cmd.VarOpt("e env", envs, "Environment variables")
		volumes := &Volumes{}
		cmd.VarOpt("v volume", volumes, "Container volume mappings")
		// Docker-only options
		net := &NetworkMode{mode: "BRIDGE"}
		cmd.VarOpt("net", net, "Network Mode [Docker only]")
		params := &Parameters{}
		cmd.VarOpt("param", params, "Freeform Docker parameters [Docker only]")
		mappings := &PortMappings{}
		cmd.VarOpt("p port", mappings, "Port mappings [Docker only]")

		// TODO
		// resources
		// volumes
		// URIs
		// ...

		cmd.Action = func() {

			if *command == "" && *image == "" && *path == "" {
				cmd.PrintLongHelp()
				os.Exit(1)
			}

			profile := profile()
			tasks := []*mesos.TaskInfo{}

			if *path == "" {
				profile.With(
					config.Master(*hostname),
					config.Restart(*restart),
					config.Command(
						config.CommandOpts{
							Value: *command,
							User:  *user,
							Shell: *shell,
							Envs:  *envs,
						},
					),
					config.Container(
						config.ContainerOpts{
							Docker:       *docker,
							Privileged:   *privileged,
							Image:        *image,
							Parameters:   *params,
							Volumes:      *volumes,
							NetworkMode:  net.Mode(),
							PortMappings: *mappings,
						},
					),
				)
				tasks = append(tasks, profile.Task())
			} else {
				infos, err := config.TasksFromFile(*path)
				failOnErr(err)
				for _, info := range infos {
					tasks = append(tasks, info)
				}
			}

			if *dump {
				raw, err := json.MarshalIndent(tasks, " ", " ")
				failOnErr(err)
				fmt.Println(string(raw))
				os.Exit(0)
			}
			failOnErr(runner.Run(profile, tasks))
		}
	}
}

type NetworkMode struct {
	mode string
}

func (n *NetworkMode) Set(v string) error {
	_, ok := mesos.ContainerInfo_DockerInfo_Network_value[strings.ToUpper(v)]
	if !ok {
		return fmt.Errorf("Bad network mode: %s", v)
	}
	n.mode = strings.ToUpper(v)
	return nil
}

func (n NetworkMode) String() string {
	return n.mode
}

func (n NetworkMode) Mode() mesos.ContainerInfo_DockerInfo_Network {
	m, _ := mesos.ContainerInfo_DockerInfo_Network_value[n.mode]
	return mesos.ContainerInfo_DockerInfo_Network(m)
}

type PortMappings []mesos.ContainerInfo_DockerInfo_PortMapping

func (mappings *PortMappings) Set(v string) (err error) {
	split := strings.Split(v, ":")
	if len(split) != 2 {
		return fmt.Errorf("Bad port mapping %s", v)
	}
	mapping := mesos.ContainerInfo_DockerInfo_PortMapping{}
	host, err := strconv.ParseUint(split[0], 0, 32)
	if err != nil {
		return fmt.Errorf("Bad port mapping %s", v)
	}
	mapping.HostPort = uint32(host)
	split = strings.Split(split[1], "/")
	if len(split) == 2 {
		split[1] = strings.ToLower(split[1])
		if !(split[1] == "tcp" || split[1] == "udp") {
			return fmt.Errorf("Bad port mapping %s", v)
		}
		mapping.Protocol = proto.String(split[1])
	}
	cont, err := strconv.ParseUint(split[0], 0, 32)
	if err != nil {
		return fmt.Errorf("Bad port mapping %s", v)
	}
	mapping.ContainerPort = uint32(cont)
	*mappings = append(*mappings, mapping)
	return nil
}

func (mappings PortMappings) String() string {
	var s string
	for _, mapping := range mappings {
		s += fmt.Sprintf("%d:%d/%s", mapping.HostPort, mapping.ContainerPort, *mapping.Protocol)
	}
	return s
}

type Volumes []mesos.Volume

func (vols *Volumes) Set(v string) error {
	// TODO Need to support image and other parameters
	split := strings.Split(v, ":")
	if len(split) < 2 {
		return fmt.Errorf("Bad volume: %s", v)
	}
	vol := mesos.Volume{
		HostPath:      split[0],
		ContainerPath: split[1],
	}
	if len(split) == 3 {
		split[2] = strings.ToUpper(split[2])
		if !(split[2] == "RW" || split[2] == "RO") {
			return fmt.Errorf("Bad volume: %s", v)
		}
		vol.Mode = mesos.Volume_Mode(mesos.Volume_Mode_value[split[2]]).Enum()
	} else {
		vol.Mode = mesos.RO.Enum()
	}
	*vols = append(*vols, vol)
	return nil
}

func (vols Volumes) String() string {
	var s string
	for _, vol := range vols {
		s += fmt.Sprintf("%s:%s", vol.HostPath, vol.ContainerPath)
	}
	return s
}

type Parameters []mesos.Parameter

func (params Parameters) String() string {
	var s string
	for _, param := range params {
		s += fmt.Sprintf("%s=%s", param.Key, param.Value)
	}
	return s
}

func (params *Parameters) Set(v string) error {
	split := strings.Split(v, "=")
	if len(split) != 2 {
		return fmt.Errorf("Bad Docker parameter %s", v)
	}
	*params = append(*params, mesos.Parameter{Key: split[0], Value: split[1]})
	return nil
}

type Envs []mesos.Environment_Variable

func (envs Envs) String() string {
	var s string
	for _, env := range envs {
		s += fmt.Sprintf("%s=%s", env.Name, env.Value)
	}
	return s
}

func (envs *Envs) Set(v string) error {
	split := strings.Split(v, "=")
	if len(split) != 2 {
		return fmt.Errorf("Bad environment variable %s", v)
	}
	*envs = append(*envs, mesos.Environment_Variable{Name: split[0], Value: split[1]})
	return nil
}
