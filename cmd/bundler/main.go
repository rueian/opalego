package main

import (
	"encoding/json"
	"fmt"
	"github.com/rueian/opalego/pkg/bundle"
	"github.com/rueian/opalego/pkg/lego"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	OutPath    string
	FetchURL   string
	Interval   time.Duration
	ConfigPath string
)

func main() {
	var rootCmd = &cobra.Command{Short: "OPA bundle maker from remote path or local json"}
	rootCmd.PersistentFlags().StringVarP(&ConfigPath, "config", "c", "", "factory config path")
	rootCmd.MarkPersistentFlagRequired("config")
	rootCmd.PersistentFlags().StringVarP(&OutPath, "out", "o", "", "/tmp/bundle.tar.gz")
	rootCmd.MarkPersistentFlagRequired("out")
	rootCmd.PersistentFlags().StringVarP(&FetchURL, "url", "u", "", "http://xxx | fs:///tmp/data.json")
	rootCmd.MarkPersistentFlagRequired("url")
	rootCmd.PersistentFlags().DurationVarP(&Interval, "interval", "i", 10*time.Second, "re-fetch interval")
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		bs, err := ioutil.ReadFile(ConfigPath)
		if err != nil {
			return err
		}

		factory := bundle.Factory{}
		if err = json.Unmarshal(bs, &factory); err != nil {
			return err
		}

		bundler := lego.NewLego(factory, lego.WithSidecar(lego.SidecarOPA{
			BundleDst: OutPath,
		}))

		bundler.ScheduleSetBundle(&fetcher{URL: FetchURL}, Interval, func(err error) {
			log.Println("fetch err", err.Error())
		})

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		return nil
	}
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

type fetcher struct {
	URL string
}

func (f *fetcher) Fetch() (data bundle.Service, err error) {
	var bs []byte
	if strings.HasPrefix(f.URL, "fs://") {
		bs, err = ioutil.ReadFile(strings.TrimPrefix(f.URL, "fs://"))
	} else {
		var resp *http.Response
		if resp, err = http.Get(f.URL); err != nil {
			return
		}
		defer resp.Body.Close()
		if bs, err = ioutil.ReadAll(resp.Body); err != nil {
			return
		}
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("remote err: %d: %s", resp.StatusCode, bs)
		}
	}
	if err == nil {
		err = json.Unmarshal(bs, &data)
	}
	return
}
