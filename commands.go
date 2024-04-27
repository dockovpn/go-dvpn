/*
 * Copyright (c) 2024. Dockovpn Solutions OÜ
 */

package go_dvpn

type Command []string

type cmds struct {
	version     Command
	genclient   Command
	rmclient    Command
	getclient   Command
	listclients Command
}

var Commands cmds

func init() {
	Commands = makeCommands()
}

func (c cmds) GetClient(clientId string) Command {
	return append(c.getclient, clientId)
}

func (c cmds) ListClients() Command {
	return c.listclients
}

func (c cmds) Version() Command {
	return c.version
}

func (c cmds) GenClient() Command {
	return append(c.genclient, "o")
}

func (c cmds) GenClientWithID(clientId string) Command {
	return append(c.genclient, "n", clientId)
}

func (c cmds) RmClient(clientId string) Command {
	return append(c.rmclient, clientId)
}

func makeCommands() cmds {
	return cmds{
		version:     []string{"./version.sh"},
		genclient:   []string{"./genclient.sh"},
		rmclient:    []string{"./rmclient.sh"},
		getclient:   []string{"./getconfig.sh"},
		listclients: []string{"./listconfigs.sh"},
	}
}
