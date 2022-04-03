package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/libdns/libdns"
	"github.com/libdns/namesilo"
)

func main() {
	token := "dc75366429d1fc8f36f9"

	zone := "matjes.life"

	provider := namesilo.Provider{
		APIToken: token,
	}

	r := libdns.Record{
		Type:  "TXT",
		Name:  "_test.matjes.life",
		Value: "text",
		TTL:   time.Duration(3600) * time.Second,
	}

	q := libdns.Record{
		Type:  "TXT",
		Name:  "_acme.matjes.life",
		Value: "token",
		TTL:   time.Duration(3600) * time.Second,
	}

	// _, err = provider.AppendRecords(context.TODO(), zone, []libdns.Record{record})
	// if err != nil {
	// 	log.Fatalln("ERROR: ", err.Error())
	// }

	_, err := provider.DeleteRecords(context.TODO(), zone, []libdns.Record{r, q})
	if err != nil {
		log.Fatalln("Deletion Error: ", err.Error())
	}

	// _, err := provider.SetRecords(context.TODO(), zone, []libdns.Record{r, q})
	// if err != nil {
	// 	log.Fatalln("ERROR: ", err.Error())
	// }

	records, err := provider.GetRecords(context.TODO(), zone)
	if err != nil {
		log.Fatalln("ERROR: ", err.Error())
	}

	for _, record := range records {
		fmt.Printf("%s (%s): %s, %s\n", record.Name, record.ID, record.Value, record.Type)
	}
}
