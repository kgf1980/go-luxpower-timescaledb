package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/joho/godotenv/autoload"
	"github.com/kgf1980/go-luxpower-timescaledb/internal/config"
	"golang.org/x/net/publicsuffix"
)

type LiveData struct {
	PhotoVoltaic1Watts     int `json:"ppv1"`
	PhotoVoltaic2Watts     int `json:"ppv2"`
	PhotoVoltaic3Watts     int `json:"ppv3"`
	PhotoVoltaicTotalWatts int `json:"ppv"`
	InverterToBattery      int `json:"pCharge"`
	BatteryToInverter      int `json:"pDisCharge"`
	BatteryChargePercent   int `json:"soc"`
	InverterToLoad         int `json:"pinv"`
	GridToLoad             int `json:"pToUser"`
	InverterToGrid         int `json:"pToGrid"`
}

type LiveDataDisplay LiveData

func (ldd LiveDataDisplay) MarshalJSON() ([]byte, error) {
	lddVal := reflect.ValueOf(ldd)
	kvpairs := []string{}

	for i := 0; i < lddVal.NumField(); i++ {
		k := lddVal.Type().Field(i).Name
		v := lddVal.Field(i).Interface()
		kvpairs = append(kvpairs, fmt.Sprintf("\"%s\":%#v", k, v))
	}

	return []byte(fmt.Sprintf("{%s}", strings.Join(kvpairs, ","))), nil
}

type LuxpowerDownload struct {
	Config     *config.Config
	Jar        *cookiejar.Jar
	Client     *http.Client
	Connection *pgx.Conn
}

func (ld *LuxpowerDownload) authenticate() error {
	v := url.Values{
		"account":  {ld.Config.AccountName},
		"password": {ld.Config.Password},
	}
	postUrl, err := url.Parse(fmt.Sprintf("%s/web/login", ld.Config.BaseUrl))
	if err != nil {
		return err
	}
	_, err = ld.Client.PostForm(postUrl.String(), v)
	if err != nil {
		return err
	}

	return nil
}

func (ld *LuxpowerDownload) GetLiveData(test_mode bool) (*LiveData, error) {
	if test_mode {
		return &LiveData{}, nil
	}
	if len(ld.Jar.Cookies(ld.Config.BaseUrl)) == 0 {
		err := ld.authenticate()
		if err != nil {
			return &LiveData{}, err
		}
	}
	liveUrl, err := url.Parse(fmt.Sprintf("%s/api/inverter/getInverterRuntime", ld.Config.BaseUrl.String()))
	if err != nil {
		return &LiveData{}, err
	}

	v := url.Values{
		"serialNum": {ld.Config.StationNumber},
	}

	r, err := ld.Client.PostForm(liveUrl.String(), v)
	if err != nil {
		return &LiveData{}, err
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return &LiveData{}, err
	}
	var data LiveData
	if err := json.Unmarshal(body, &data); err != nil {
		return &LiveData{}, err
	}
	return &data, nil
}

func (ld *LuxpowerDownload) UpdateInverterData() error {
	log.Println("Fetching data from LuxPower")
	data, err := ld.GetLiveData(false)
	if err != nil {
		return err
	}

	_, err = ld.Connection.Exec(context.Background(), `INSERT INTO inverter_data (time, station_number, battery_charge_percent, pv_1, pv_2, pv_3, pv_total, battery_charge, battery_discharge, inverter_to_load, inverter_to_grid, grid_to_load, load)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		time.Now(), ld.Config.StationNumber, data.BatteryChargePercent, data.PhotoVoltaic1Watts, data.PhotoVoltaic2Watts, data.PhotoVoltaic3Watts, data.PhotoVoltaicTotalWatts,
		data.InverterToBattery, data.BatteryToInverter, data.InverterToLoad, data.InverterToGrid, data.GridToLoad, data.InverterToLoad+data.GridToLoad)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	conn, err := pgx.Connect(context.Background(), cfg.DatabaseUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(context.Background())
	err = conn.Ping(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	MigrateDb(conn)

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar: jar,
	}

	d := LuxpowerDownload{
		Config:     cfg,
		Jar:        jar,
		Client:     client,
		Connection: conn,
	}

	d.UpdateInverterData()
}

func MigrateDb(conn *pgx.Conn) error {
	ctx := context.Background()
	_, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS timescaledb;")
	if err != nil {
		return err
	}

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.inverter_data (
		"time" timestamp with time zone NOT NULL,
		station_number text NOT NULL,
		battery_charge_percent integer NOT NULL,
		pv_1 integer NOT NULL,
		pv_2 integer NOT NULL,
		pv_3 integer NOT NULL,
		pv_total integer NOT NULL,
		battery_charge integer NOT NULL,
		battery_discharge integer NOT NULL,
		inverter_to_load integer NOT NULL,
		inverter_to_grid integer NOT NULL,
		grid_to_load integer NOT NULL,
		load integer NOT NULL
		);`)
	if err != nil {
		return err
	}
	row := conn.QueryRow(ctx, "SELECT COUNT(*) FROM timescaledb_information.hypertables WHERE hypertable_name = 'inverter_data';")
	var rowCount int
	err = row.Scan(&rowCount)
	if err != nil {
		return err
	}
	if rowCount == 0 {
		_, err := conn.Exec(ctx, "SELECT create_hypertable('inverter_data', by_range('time'));")
		if err != nil {
			return err
		}
	}

	_, err = conn.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS ix_inverter_time_station ON inverter_data (time, station_number);`)
	if err != nil {
		return err
	}

	//_, err = conn.Exec(ctx, `ALTER TABLE inverter_data ADD UNIQUE(time, station_number);`)
	//if err != nil {
	//	return err
	//}
	return nil
}
