/*
 * Copyright (c) 2024. Dockovpn Solutions OÜ
 */

package go_dvpn

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	dockovpnInitializationCompleted = "Initialization Sequence Completed"
)

type dockovpnType struct {
	containerId  string
	created      bool
	started      bool
	initialized  bool
	dockerClient *client.Client
	mtx          sync.Mutex
}

type DvpnInterface interface {
	Start(dvpnContainerOpts DvpnContainerOptions, registryCreds RegistryCreds, opts StartOptions) (VolumeRemoveHandle, error)
	StartWithPersistentVolume(dvpnContainerOpts DvpnContainerOptions, volumeName string, registryCreds RegistryCreds, opts StartOptions) (VolumeRemoveHandle, error)
	GetClient(clientId string) (string, error)
	ListClients() (string, error)
	Version() (string, error)
	GenerateClient() (string, error)
	GenerateClientWithID(clientId string) (string, error)
	RemoveClient(clientId string) (string, error)
	Close()
}

func FindInstance(containerName string) DvpnInterface {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	d := &dockovpnType{
		dockerClient: cli,
	}

	dvpnContainer, err := d.findContainerByName(containerName)
	if err != nil {
		return nil
	}

	fmt.Println("found exiting dockovpn container")
	d.containerId = dvpnContainer.ID
	d.created = true
	d.started = true
	d.initialized = true

	return d
}

func NewDockovpn() DvpnInterface {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	fmt.Println("Created docker client")

	return &dockovpnType{
		dockerClient: cli,
	}
}

func (d *dockovpnType) Start(dvpnContainerOpts DvpnContainerOptions, registryCreds RegistryCreds, opts StartOptions) (VolumeRemoveHandle, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	ctx := context.Background()

	return nil, d.startInternal(ctx, dvpnContainerOpts, nil, registryCreds, opts)
}

func (d *dockovpnType) StartWithPersistentVolume(dvpnContainerOpts DvpnContainerOptions, volumeName string, registryCreds RegistryCreds, opts StartOptions) (VolumeRemoveHandle, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	ctx := context.Background()
	found, _ := d.findVolumeByName(ctx, volumeName)

	var vol *volume.Volume
	var volumeHandle VolumeRemoveHandle
	if found == nil {
		created, err := d.createVolumeWithName(ctx, volumeName)
		if err != nil {
			fmt.Printf("could not create volume [%s]", volumeName)
		}
		vol = created
		volumeHandle = NewVolumeRemoveHandle(d, d.dockerClient, vol.Name)
	} else {
		vol = found
	}

	return volumeHandle, d.startInternal(ctx, dvpnContainerOpts, vol, registryCreds, opts)
}

func (d *dockovpnType) startInternal(ctx context.Context, dvpnContainerOpts DvpnContainerOptions, vol *volume.Volume, registryCreds RegistryCreds, opts StartOptions) error {
	if d.started {
		return errors.New("container has already started")
	}

	cli := d.dockerClient

	reader, err := cli.ImagePull(ctx, dvpnContainerOpts.ImageUrl, types.ImagePullOptions{
		RegistryAuth: GetAuthToken(registryCreds),
	})
	if err != nil {
		return err
	}

	defer reader.Close()
	io.Copy(os.Stdout, reader)

	cmd := parseOptions(opts)

	containerConfig := &container.Config{
		Image: dvpnContainerOpts.ImageUrl,
		Tty:   true,
		Cmd:   strslice.StrSlice(cmd),
	}

	portBindings := makePortMap(false)
	mounts := makeDockovpnDataVolumeMount(vol)

	hostConfig := &container.HostConfig{
		CapAdd:       []string{"NET_ADMIN"},
		PortBindings: portBindings,
		Mounts:       mounts,
		AutoRemove:   true,
	}

	resp, err := cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		dvpnContainerOpts.ContainerName,
	)
	if err != nil {
		return err
	}

	d.containerId = resp.ID
	d.created = true

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return err
	}

	d.started = true

	var ctxTimeout time.Duration
	if opts.Regenerate {
		ctxTimeout = 5 * time.Minute
	} else {
		ctxTimeout = 10 * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()

	logsReader, err := cli.ContainerLogs(timeoutCtx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}

	defer logsReader.Close()

	scanner := bufio.NewScanner(logsReader)
	for scanner.Scan() {
		line := scanner.Text()
		println(line)
		if strings.Contains(line, dockovpnInitializationCompleted) {
			d.initialized = true
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (d *dockovpnType) GetClient(clientId string) (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/getclient.sh")

	res, err := d.execInContainer(Commands.GetClient(clientId))
	return res, err
}

func (d *dockovpnType) ListClients() (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/listconfigs.sh")

	res, err := d.execInContainer(Commands.ListClients())
	return res, err
}

func (d *dockovpnType) Version() (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/version.sh")

	res, err := d.execInContainer(Commands.Version())
	out := CleanString(res)
	resParts := strings.Split(out, " ")
	ver := resParts[len(resParts)-1]
	return ver, err
}

func (d *dockovpnType) GenerateClient() (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/genclient.sh")

	res, err := d.execInContainer(Commands.GenClient())
	return res, err
}

func (d *dockovpnType) GenerateClientWithID(clientId string) (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/genclient.sh n " + clientId)

	res, err := d.execInContainer(Commands.GenClientWithID(clientId))
	return res, err
}

func (d *dockovpnType) RemoveClient(clientId string) (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	println("exec/rmclient.sh")

	res, err := d.execInContainer(Commands.RmClient(clientId))
	return res, err
}

func (d *dockovpnType) Close() {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	fmt.Printf("removing container [%s]\n", d.containerId)

	d.removeContainer()
}

func (d *dockovpnType) findVolumeByName(ctx context.Context, volumeName string) (*volume.Volume, error) {
	cli := d.dockerClient

	resp, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, v := range resp.Volumes {
		if v.Name == volumeName {
			return v, nil
		}
	}

	return nil, fmt.Errorf("volume [%s] not found", volumeName)
}

func (d *dockovpnType) findContainerByName(containerName string) (*types.Container, error) {
	cli := d.dockerClient
	ctx := context.Background()
	filterArgs := filters.NewArgs()
	filterArgs.Add("name", containerName)

	resp, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	if len(resp) == 1 {
		return &resp[0], nil
	} else if len(resp) > 1 {
		return nil, fmt.Errorf("more than 1 container with name [%s] found", containerName)
	}

	return nil, fmt.Errorf("container [%s] not found", containerName)
}

func (d *dockovpnType) createVolumeWithName(ctx context.Context, volumeName string) (*volume.Volume, error) {
	cli := d.dockerClient

	fmt.Printf("creating volume [%s]\n", volumeName)

	vol, err := cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: volumeName,
	})
	if err != nil {
		return nil, err
	}

	return &vol, nil
}

func (d *dockovpnType) execInContainer(cmd Command) (string, error) {
	if !d.initialized {
		return "", errors.New("dockovpn isn't initialized")
	}

	ctx := context.Background()
	cli := d.dockerClient
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := cli.ContainerExecCreate(ctx, d.containerId, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: false,
		Tty:          true,
		Cmd:          cmd,
	})
	if err != nil {
		return "", err
	}

	hijackedResp, err := cli.ContainerExecAttach(ctxTimeout, resp.ID, types.ExecStartCheck{
		Tty: true,
	})
	if err != nil {
		return "", err
	}
	defer hijackedResp.Close()

	execOutput, err := io.ReadAll(hijackedResp.Reader)
	if err != nil {
		return "", err
	}

	clean := string(execOutput[:])

	inspectResp, err := cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		println("error in exec inspect...")
		return clean, err
	}

	println("exec exit code: ", inspectResp.ExitCode)

	if inspectResp.ExitCode != 0 {
		return clean, errors.New("operation wasn't successful")
	}

	return clean, nil
}

func (d *dockovpnType) removeContainer() {
	if !d.started {
		return
	}
	ctx := context.Background()
	cli := d.dockerClient

	err := cli.ContainerRemove(ctx, d.containerId, container.RemoveOptions{
		Force: true,
	})
	if err != nil {
		fmt.Printf("error [%s] removing container [%s]", err, d.containerId)
		return
	}

	d.resetState()
}

func (d *dockovpnType) resetState() {
	d.created = false
	d.started = false
	d.initialized = false
}

func parseOptions(opts StartOptions) Command {
	var cmd []string

	if opts.Regenerate {
		cmd = append(cmd, "-r")
	}

	if opts.Skip {
		cmd = append(cmd, "-s")
	}

	if opts.Noop {
		cmd = append(cmd, "-n")
	}

	if opts.Quit {
		cmd = append(cmd, "-q")
	}

	return cmd
}

func makeDockovpnDataVolumeMount(vol *volume.Volume) []mount.Mount {
	if vol != nil {
		return []mount.Mount{
			{
				Source:   vol.Name,
				Target:   "/opt/Dockovpn_data",
				Type:     "volume",
				ReadOnly: false,
			},
		}
	} else {
		return nil
	}
}

// TODO: Add OpenVPN management port
func makePortMap(tcp bool) nat.PortMap {
	portMap := nat.PortMap{
		"1194/udp": {
			{
				HostIP:   "0.0.0.0",
				HostPort: "1194/udp",
			},
		},
	}

	if tcp {
		portMap["8080/tcp"] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: "80/tcp",
			},
		}
	}

	return portMap
}
