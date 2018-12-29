package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	floats "gonum.org/v1/gonum/floats"
	stat "gonum.org/v1/gonum/stat"
	fast "gopkg.in/ddo/go-fast.v0"
)

const (
	// AppName is this application's name.
	AppName = `bwmon`

	// Version is this application's semantic version string.
	Version = `0.0.1`
)

// Config is all of the configurable knobs for this application.
type Config struct {
	Debug       bool
	Interval    int
	Measurement string
	Tags        map[string]string
	Fields      map[string]string
	DBURL       string
}

// Point is a JSON marshalable data point.
type Point struct {
	Measurement string
	Time        time.Time
	Fields      map[string]string
	Tags        map[string]string
}

// NewConfig returns a new Config.
func NewConfig() *Config {
	c := &Config{}

	c.Fields = make(map[string]string, 0)
	c.Tags = make(map[string]string, 0)

	return c
}

// NewPoint returns a new point.
func NewPoint(c *Config) *Point {
	p := &Point{}

	p.Time = time.Now()
	p.Measurement = c.Measurement

	p.Tags = make(map[string]string)
	for k, v := range c.Tags {
		p.Tags[k] = v
	}

	p.Fields = make(map[string]string)
	for k, v := range c.Fields {
		p.Fields[k] = v
	}

	return p
}

// Line creates an influxdb line protocol string from a given data
// point.
func (p *Point) Line() string {
	tags := strings.Builder{}
	for k, v := range p.Tags {
		tags.WriteRune(',')
		tags.WriteString(k)
		tags.WriteRune('=')
		tags.WriteString(v)
	}

	fields := make([]string, 0, len(p.Fields))
	for k, v := range p.Fields {
		fields = append(fields, fmt.Sprintf("%s=%s", k, v))
	}

	return fmt.Sprintf("%s%s %s %d",
		p.Measurement,
		tags.String(),
		strings.Join(fields, ","),
		p.Time.UnixNano())
}

func measure(config *Config) error {
	f := fast.New()
	if err := f.Init(); err != nil {
		return err
	}

	urls, err := f.GetUrls()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	kbpsChan := make(chan float64)

	wg.Add(1)
	go func() {
		points := make([]float64, 0, 0)
		for kbps := range kbpsChan {
			points = append(points, kbps)
		}

		mean, stddev := stat.MeanStdDev(points, nil)

		point := NewPoint(config)
		point.Fields["min"] = strconv.FormatFloat(floats.Min(points), 'f', -1, 64)
		point.Fields["max"] = strconv.FormatFloat(floats.Max(points), 'f', -1, 64)
		point.Fields["mean"] = strconv.FormatFloat(mean, 'f', -1, 64)
		point.Fields["stddev"] = strconv.FormatFloat(stddev, 'f', -1, 64)

		if err := write(config, point); err != nil {
			log.Printf("Error writing line: %s", err.Error())
		}

		wg.Done()
	}()

	if err := f.Measure(urls, kbpsChan); err != nil {
		return err
	}

	wg.Wait()

	return nil
}

func write(c *Config, p *Point) error {
	line := p.Line()

	body := strings.NewReader(line)
	res, err := http.Post(c.DBURL, "application/octet-stream", body)
	if err != nil {
		return err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			body = []byte("Unable to read body.")
		}
		return fmt.Errorf("Influxdb returned %s: %#v", res.Status, string(body))
	}

	if c.Debug {
		log.Printf("%s: %s", res.Status, line)
	}

	return nil
}

func setMetadata(c *Config) {
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetPrefix(AppName + ` `)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	config := NewConfig()

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf(err.Error())
	}

	config.Tags[`Hostname`] = hostname
	config.Tags[`AppName`] = AppName
	config.Tags[`Version`] = Version

	flag.BoolVar(&config.Debug, "d", false, "Log debugging messages.")
	flag.IntVar(&config.Interval, "i", 300, "Measurement interval in seconds.")
	flag.StringVar(&config.Measurement, "m", "bandwidth", "Measurement name.")
	flag.StringVar(&config.DBURL, "u", "http://127.0.0.1:8086/write?db=bwmon", "InfluxDB URL")
	flag.Parse()

	if config.Debug {
		log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
		log.Printf(`%s %s`, AppName, Version)
	}

	for {
		if err := measure(config); err != nil {
			log.Printf(err.Error())
			continue
		}

		time.Sleep(time.Duration(config.Interval) * time.Second)
	}
}
