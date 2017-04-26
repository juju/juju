package cmd

func ListCharmResourcesCommandChannel(c *ListCharmResourcesCommand) string {
	return c.channel
}

func ShowServiceCommandTarget(c *ShowServiceCommand) string {
	return c.target
}

func UploadCommandResourceFile(c *UploadCommand) (service, name, filename string) {
	return c.resourceFile.service,
		c.resourceFile.name,
		c.resourceFile.filename
}

func UploadCommandService(c *UploadCommand) string {
	return c.service
}

var FormatServiceResources = formatServiceResources
