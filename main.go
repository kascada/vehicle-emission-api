package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kamran/vehicle-emission-api/cache"
	"github.com/kamran/vehicle-emission-api/client"
	"github.com/kamran/vehicle-emission-api/handler"
	"github.com/kamran/vehicle-emission-api/validator"
)

// --- CLI dispatcher ---

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api vehicle [flags] <id>   Fahrzeugdaten abrufen")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api validate-email <email> E-Mail-Adresse prüfen")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api check-email <email>    E-Mail prüfen (mit Cache)")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api fetch <id> <email>     Fahrzeug + E-Mail-Check")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api serve                  HTTP-Server starten")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "flags (vehicle, fetch):")
	fmt.Fprintln(os.Stderr, "  -text     Lesbare Textausgabe statt JSON")
	fmt.Fprintln(os.Stderr, "  -verbose  Aufgerufene URLs, Status-Codes, Datenwarnungen")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "vehicle":
		err = cmdVehicle(args)
	case "validate-email":
		err = cmdValidateEmail(args)
	case "check-email":
		err = cmdCheckEmail(args)
	case "fetch":
		err = cmdFetch(args)
	case "serve":
		err = cmdServe(args)
	default:
		fmt.Fprintf(os.Stderr, "unbekannter Befehl: %q\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		os.Exit(1)
	}
}

func cmdVehicle(args []string) error {
	fs := flag.NewFlagSet("vehicle", flag.ContinueOnError)
	textMode := fs.Bool("text", false, "Lesbare Textausgabe statt JSON")
	verboseFlag := fs.Bool("verbose", false, "Aufgerufene URLs, Status-Codes, Datenwarnungen")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("verwendung: vehicle [-text] [-verbose] <id>")
	}
	id := fs.Arg(0)
	if n, err := strconv.Atoi(id); err != nil || n <= 0 {
		return fmt.Errorf("vehicle-id muss ein positiver Integer sein, got %q", id)
	}

	fc := client.NewFuelEconomyClient(nil, nil, *verboseFlag)
	vehicle, err := fc.GetVehicle(id)
	if err != nil {
		return err
	}

	if *textMode {
		printVehicleText(vehicle)
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(vehicle)
}

func cmdValidateEmail(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: validate-email <email>")
		os.Exit(1)
	}
	email := args[0]
	v := validator.NewEmailValidator(validator.NewDisposableChecker())
	if err := v.Validate(email); err != nil {
		if strings.Contains(err.Error(), "disposable") {
			fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
		} else {
			fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("OK: %s\n", email)
	return nil
}

func cmdCheckEmail(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: check-email <email>")
		os.Exit(1)
	}
	email := args[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := cache.NewEmailCache(time.Hour, ctx)

	if c.IsVerified(email) {
		fmt.Printf("CACHED: %s\n", email)
		return nil
	}

	v := validator.NewEmailValidator(validator.NewDisposableChecker())
	if err := v.Validate(email); err != nil {
		if strings.Contains(err.Error(), "disposable") {
			fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
		} else {
			fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		}
		os.Exit(1)
	}

	c.Add(email)
	fmt.Printf("OK: %s (added to cache)\n", email)
	return nil
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	textMode := fs.Bool("text", false, "Lesbare Textausgabe statt JSON")
	verboseMode := fs.Bool("verbose", false, "Verbose-Ausgabe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: fetch [-text] [-verbose] <vehicle-id> <email>")
		os.Exit(1)
	}
	vehicleID := fs.Arg(0)
	email := fs.Arg(1)

	vlog := func(prefix, msg string) {
		if *verboseMode {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", prefix, msg)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// E-Mail: Cache prüfen, ggf. validieren
	c := cache.NewEmailCache(time.Hour, ctx)
	if c.IsVerified(email) {
		vlog("cache", "HIT: "+email)
	} else {
		vlog("cache", "MISS: "+email+" — validating...")
		ev := validator.NewEmailValidator(validator.NewDisposableChecker())
		if err := ev.Validate(email); err != nil {
			if strings.Contains(err.Error(), "disposable") {
				fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
			} else {
				fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
			}
			os.Exit(1)
		}
		c.Add(email)
		vlog("cache", "ADDED: "+email)
	}

	// Vehicle-ID: positiver Integer
	id, err := strconv.Atoi(vehicleID)
	if err != nil || id <= 0 {
		return fmt.Errorf("vehicle-id muss ein positiver Integer sein, got %q", vehicleID)
	}

	vc := cache.NewVehicleCache(10000, ctx)
	fc := client.NewFuelEconomyClient(nil, vc, *verboseMode)

	vlog("api", "fetching vehicle "+vehicleID+"...")
	vehicle, err := fc.GetVehicle(vehicleID)
	if err != nil {
		return err
	}
	vlog("api", "OK (200)")

	if *textMode {
		printVehicleText(vehicle)
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(vehicle)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.String("port", "8081", "Port auf dem der Server lauscht")
	verboseMode := fs.Bool("verbose", false, "Cache-Treffer und -Misses auf stderr ausgeben")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	vc := cache.NewVehicleCache(10000, ctx)
	fc := client.NewFuelEconomyClient(nil, vc, *verboseMode)
	ev := validator.NewEmailValidator(validator.NewDisposableChecker())
	ec := cache.NewEmailCache(6*time.Hour, ctx)
	rl := cache.NewRateLimiter(1000, time.Minute, ctx)
	h := handler.New(ev, ec, rl, fc, *verboseMode)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /vehicle/{id}", h.GetVehicle)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found","usage":"GET /vehicle/{id}?email=user@example.com"}`))
	})

	addr := ":" + *port
	fmt.Fprintf(os.Stderr, "[serve] listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func printVehicleText(v *client.VehicleData) {
	co2Str := "n/a"
	if v.CO2 != nil {
		co2Str = fmt.Sprintf("%.1f g/mi", *v.CO2)
	}
	fmt.Printf("%s %s (%d)\n", v.Make, v.Model, v.Year)
	fmt.Printf("Fuel: %s\n", v.FuelType)
	fmt.Printf("City: %d | Highway: %d | Combined: %d\n", v.City08, v.Highway08, v.Comb08)
	fmt.Printf("CO2: %s\n", co2Str)
	fmt.Printf("Class: %s\n", v.VClass)
}

func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return email
}
