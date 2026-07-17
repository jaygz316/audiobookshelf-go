package main

import (
	"flag"
	"os"
	"path"
	"path/filepath"

	"audiobookshelf/internal/core"
)

func parseConfig() *core.Config {
	configFlag := flag.String("c", "", "Config path")
	metadataFlag := flag.String("m", "", "Metadata path")
	portFlag := flag.String("p", "", "Port")
	hostFlag := flag.String("h", "", "Host")
	sourceFlag := flag.String("s", "", "Source")
	devFlag := flag.Bool("d", false, "Dev mode")
	prodDevFlag := flag.Bool("r", false, "Prod with dev env")
	legacyURLFlag := flag.String("legacy-url", "http://localhost:3334", "Legacy Node.js server URL")

	flag.Parse()

	configPath := *configFlag
	if configPath == "" {
		configPath = os.Getenv("CONFIG_PATH")
	}
	if configPath == "" {
		configPath = "config"
	}
	configPath, _ = filepath.Abs(configPath)

	metadataPath := *metadataFlag
	if metadataPath == "" {
		metadataPath = os.Getenv("METADATA_PATH")
	}
	if metadataPath == "" {
		metadataPath = "metadata"
	}
	metadataPath, _ = filepath.Abs(metadataPath)

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "3333"
	}

	host := *hostFlag
	if host == "" {
		host = os.Getenv("HOST")
	}

	source := *sourceFlag
	if source == "" {
		source = os.Getenv("SOURCE")
	}
	if source == "" {
		source = "debian"
	}

	routerBasePath, exists := os.LookupEnv("ROUTER_BASE_PATH")
	if !exists {
		routerBasePath = "/audiobookshelf"
	}
	routerBasePath = path.Clean("/" + routerBasePath)

	return &core.Config{
		ConfigPath:     configPath,
		MetadataPath:   metadataPath,
		Port:           port,
		Host:           host,
		Source:         source,
		Dev:            *devFlag,
		ProdWithDevEnv: *prodDevFlag,
		LegacyURL:      *legacyURLFlag,
		RouterBasePath: routerBasePath,
	}
}
