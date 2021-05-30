package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	mtbl "github.com/hdm/golang-mtbl"
	"github.com/hdm/inetdata-parsers"
)

func usage() {
	fmt.Println("Usage: " + os.Args[0] + " [options]")
	fmt.Println("")
	fmt.Println("Creates a MTBL database from a CSV input.")
	fmt.Println("")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func mergeFunc(key []byte, val0 []byte, val1 []byte) (mergedVal []byte) {
	return []byte(string(val0) + " " + string(val1))
}

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())
	os.Setenv("LC_ALL", "C")

	flag.Usage = func() { usage() }

	indexKey := flag.Int("k", 1, "The field index to use as the key")
	indexVal := flag.Int("v", 2, "The field index to use as the value")
	reverseKey := flag.Bool("r", false, "Store the key in reverse order")
	maxFields := flag.Int("M", -1, "The maximum number of fields to parse with the delimiter")
	compression := flag.String("c", "snappy", "The compression type to use (none, snappy, zlib, lz4, lz4hc)")
	delimiter := flag.String("d", ",", "The delimiter to use as a field separator")
	sortSkip := flag.Bool("S", false, "Skip the sorting phase and assume keys are in pre-sorted order")
	sortTmp := flag.String("t", "", "The temporary directory to use for the sorting phase")
	sortMem := flag.Uint64("m", 1, "The maximum amount of memory to use, in gigabytes, for the sorting phase")

	version := flag.Bool("version", false, "Show the version and build timestamp")

	flag.Parse()

	if *version {
		inetdata.PrintVersion("inetdata-csv2mtbl")
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		usage()
		os.Exit(1)
	}

	fname := flag.Args()[0]

	sortOpt := mtbl.SorterOptions{Merge: mergeFunc, MaxMemory: 1000000000}
	sortOpt.MaxMemory *= *sortMem
	if len(*sortTmp) > 0 {
		sortOpt.TempDir = *sortTmp
	}

	compAlg, ok := inetdata.MTBLCompressionTypes[*compression]
	if !ok {
		fmt.Fprintf(os.Stderr, "Invalid compression algorithm: %s\n", *compression)
		os.Exit(1)
	}

	s := mtbl.SorterInit(&sortOpt)
	defer s.Destroy()

	w, we := mtbl.WriterInit(fname, &mtbl.WriterOptions{Compression: compAlg})
	defer w.Destroy()

	if we != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", we)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	// Tune Scanner's value for MaxScanTokenSize which defaults to 65,536
	// Lines longer than MaxScanTokenSize will cause the Scanner to fail
	// Set the intial buffsize to twice the default
	buf := make([]byte, 0, 128*1024)
	scanner.Buffer(buf, len(buf)*2)

	var cline uint = 1
	for scanner.Scan() {
		cline++
		raw := strings.TrimSpace(scanner.Text())
		if len(raw) == 0 {
			continue
		}

		bits := strings.SplitN(raw, *delimiter, *maxFields)

		if len(bits) < *indexKey {
			fmt.Fprintf(os.Stderr, "No key: %s\n", raw)
			continue
		}

		if len(bits) < *indexVal {
			fmt.Fprintf(os.Stderr, "No value: %s\n", raw)
			continue
		}

		kstr := bits[*indexKey-1]
		if len(kstr) == 0 {
			continue
		}

		vstr := bits[*indexVal-1]
		if len(vstr) == 0 {
			continue
		}

		if *reverseKey {
			kstr = inetdata.ReverseKey(kstr)
		}

		if len(kstr) > inetdata.MTBL_KEY_LIMIT || len(vstr) > inetdata.MTBL_VAL_LIMIT {
			fmt.Printf("Failed to add entry with long key or value\n")
			continue
		}

		if *sortSkip {
			if e := w.Add([]byte(kstr), []byte(vstr)); e != nil {
				fmt.Printf("Failed to add %v -> %v: %v\n", kstr, vstr, e)
			}
		} else {
			if e := s.Add([]byte(kstr), []byte(vstr)); e != nil {
				fmt.Printf("Failed to add %v -> %v: %v\n", kstr, vstr, e)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error while processing line %d : %s\n", cline, err)
		os.Exit(1)
	}

	if !*sortSkip {
		if e := s.Write(w); e != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", e)
			os.Exit(1)
		}
	}
}
