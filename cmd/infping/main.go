// infping.go copyright Tor Hveem
// License: MIT

package main

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	client "github.com/influxdata/influxdb1-client"
	toml "github.com/pelletier/go-toml"
)

func herr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func perr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func slashSplitter(c rune) bool {
	return c == '/'
}

func pingV4Hosts(config *toml.Tree, con *client.Client) {
	args := []string{"-4", "-B 1", "-D", "-r0", "-O 0", "-Q 10", "-l"}
	hosts := config.Get("hosts.v4hosts").([]interface{})
	for _, v := range hosts {
		host, _ := v.(string)
		args = append(args, host)
	}
	readPoints(hosts, args, config, con)
}

func pingV6Hosts(config *toml.Tree, con *client.Client) {
	args := []string{"-6", "-B 1", "-D", "-r0", "-O 0", "-Q 10", "-l"}
	hosts := config.Get("hosts.v6hosts").([]interface{})
	for _, v := range hosts {
		host, _ := v.(string)
		args = append(args, host)
	}
	readPoints(hosts, args, config, con)
}

// func readPoints(config *toml.Tree, con *client.Client) {
func readPoints(hosts []interface{}, args []string,
	config *toml.Tree, con *client.Client) {
	// args := []string{"-B 1", "-D", "-r0", "-O 0", "-Q 10", "-l"}
	// hosts := config.Get("hosts.hosts").([]interface{})
	fping_exec := config.Get("fping.path").(string)
	for _, v := range hosts {
		host, _ := v.(string)
		args = append(args, host)
	}
	log.Printf("Going to ping the following hosts: %q", hosts)
	cmd := exec.Command(fping_exec, args...)
	stdout, err := cmd.StdoutPipe()
	herr(err)
	stderr, err := cmd.StderrPipe()
	herr(err)
	cmd.Start()
	perr(err)

	buff := bufio.NewScanner(stderr)
	for buff.Scan() {
		text := buff.Text()
		fields := strings.Fields(text)
		// Ignore timestamp
		if len(fields) > 1 {
			fmt.Printf("fping Resp: [%s].\n", text)
			host := fields[0]
			data := fields[4]
			dataSplitted := strings.FieldsFunc(data, slashSplitter)
			// Remove ,
			dataSplitted[2] = strings.TrimRight(dataSplitted[2], "%,")
			sent, recv, lossp := dataSplitted[0], dataSplitted[1], dataSplitted[2]
			min, max, avg := "", "", ""
			// Ping times
			if len(fields) > 5 {
				times := fields[7]
				td := strings.FieldsFunc(times, slashSplitter)
				min, avg, max = td[0], td[1], td[2]
			}
			//log.Printf("Host:%s, loss: %s, min: %s, avg: %s, max: %s", host, lossp, min, avg, max)
			writePoints(config, con, host, sent, recv, lossp, min, avg, max)
		}
	}
	std := bufio.NewReader(stdout)
	line, err := std.ReadString('\n')
	perr(err)
	log.Printf("stdout:%s", line)
}

func writePoints(config *toml.Tree, con *client.Client, host string, sent string, recv string, lossp string, min string, avg string, max string) {
	db := config.Get("influxdb.db").(string)
	loss, _ := strconv.Atoi(lossp)
	pts := make([]client.Point, 1)
	fields := map[string]interface{}{}
	if min != "" && avg != "" && max != "" {
		min, _ := strconv.ParseFloat(min, 64)
		avg, _ := strconv.ParseFloat(avg, 64)
		max, _ := strconv.ParseFloat(max, 64)
		fields = map[string]interface{}{
			"loss": loss,
			"min":  min,
			"avg":  avg,
			"max":  max,
		}
	} else {
		fields = map[string]interface{}{
			"loss": loss,
		}
	}
	pts[0] = client.Point{
		Measurement: config.Get("influxdb.measurement").(string),
		Tags: map[string]string{
			"host": host,
		},
		Fields:    fields,
		Time:      time.Now(),
		Precision: "",
	}

	bps := client.BatchPoints{
		Points:          pts,
		Database:        db,
		RetentionPolicy: "autogen",
	}
	_, err := con.Write(bps)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	config, err := toml.LoadFile("config.toml")
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	influxdb_url := config.Get("influxdb.url").(string)
	username := config.Get("influxdb.user").(string)
	password := config.Get("influxdb.pass").(string)

	u, err := url.Parse(influxdb_url)
	if err != nil {
		log.Fatal(err)
	}

	conf := client.Config{
		URL:      *u,
		Username: username,
		Password: password,
	}

	con, err := client.NewClient(conf)
	if err != nil {
		log.Fatal(err)
	}

	dur, ver, err := con.Ping()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Connected to influxdb! %v, %s", dur, ver)

	// readPoints(config, con)
	go pingV4Hosts(config, con)
	go pingV6Hosts(config, con)

	fmt.Println()
}
