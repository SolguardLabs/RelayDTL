package main

import (
	"flag"
	"fmt"
	"os"

	relay "github.com/SolguardLabs/RelayDTL/src"
)

func main() {
	var scenarioPath string
	var demo bool
	var pretty bool
	flag.StringVar(&scenarioPath, "scenario", "", "path to scenario JSON")
	flag.BoolVar(&demo, "demo", false, "run built-in demo scenario")
	flag.BoolVar(&pretty, "pretty", true, "pretty-print JSON output")
	flag.Parse()

	var scenario relay.Scenario
	if demo {
		scenario = relay.DemoScenario()
	} else {
		if scenarioPath == "" {
			fmt.Fprintln(os.Stderr, "provide --scenario or --demo")
			os.Exit(2)
		}
		file, err := os.Open(scenarioPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer file.Close()
		scenario, err = relay.DecodeScenario(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

	result, err := relay.RunScenario(scenario)
	if encodeErr := relay.EncodeScenarioResult(os.Stdout, result, pretty); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr.Error())
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}
