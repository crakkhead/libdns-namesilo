// Package namesilo implements a DNS record management client compatible
// with the libdns interfaces for namesilo.
package namesilo

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// Provider facilitates DNS record manipulation with namesilo.
type Provider struct {
	APIToken string
}

func getDomain(zone string) string {
	return strings.TrimSuffix(zone, ".")
}

func getHostname(zone, name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, zone), ".")
}

func (p *Provider) getApiHost() string {
	return "https://www.namesilo.com/api"
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	log.Println("GetRecords", zone)

	client := http.Client{}

	domain := getDomain(zone)

	req, err := http.NewRequest("GET", p.getApiHost()+"/dnsListRecords?version=1&type=xml&key="+p.APIToken+"&domain="+domain, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("could not get records: Domain: %s; Status: %v; Body: %s", domain, resp.StatusCode, string(bodyBytes))
	}

	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var resultObj struct {
		Records []struct {
			ID       string `xml:"record_id"`
			Type     string `xml:"type"`
			Name     string `xml:"host"`
			Value    string `xml:"value"`
			TTL      int    `xml:"ttl"`
			Priority int    `xml:"distance"`
		} `xml:"reply>resource_record"`
	}

	err = xml.Unmarshal(result, &resultObj)
	if err != nil {
		log.Fatalf("didn't expect error: %s", err)
	}

	var records []libdns.Record

	for _, record := range resultObj.Records {
		records = append(records, libdns.Record{
			ID:       record.ID,
			Type:     record.Type,
			Name:     record.Name,
			Value:    record.Value,
			TTL:      time.Duration(record.TTL) * time.Second,
			Priority: record.Priority,
		})
	}

	return records, nil
}

//AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	log.Println("AppendRecords", zone, records)
	var appendedRecords []libdns.Record

	for _, record := range records {
		client := http.Client{}

		domain := getDomain(zone)
		host := getHostname(zone, record.Name)

		rrttl := ""
		if record.TTL != time.Duration(0) {
			rrttl = fmt.Sprintf("&rrttl=%d", int64(record.TTL/time.Second))
		}

		rrdistance := ""
		if record.Priority != 0 {
			rrdistance = fmt.Sprintf("&rrdistance=%d", record.Priority)
		}

		req_url := p.getApiHost() + "/dnsAddRecord?version=1&type=xml&key=" + p.APIToken + "&domain=" + domain + "&rrtype=" + record.Type + "&rrhost=" + host + "&rrvalue=" + record.Value + rrttl + rrdistance
		req, err := http.NewRequest("GET", req_url, nil)
		if err != nil {
			return nil, fmt.Errorf("Request error: " + p.getApiHost() + "/dnsAddRecord?version=1&type=xml&key=" + p.APIToken +
				"&domain=" + domain + "&rrtype=" + record.Type + "&rrhost=" + host + "&rrvalue=" + record.Value + rrttl)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP error: Domain: %s; Record: %s, Status: %v; Body: %s",
				getDomain(zone), getHostname(zone, record.Name), resp.StatusCode, string(bodyBytes))
		}

		var reply struct {
			Code   int    `xml:"reply>code"`
			Detail string `xml:"reply>detail"`
		}

		err = xml.Unmarshal(bodyBytes, &reply)
		if err != nil {
			return nil, fmt.Errorf("didn't expect error: %s", err)
		}

		if reply.Code != 300 {
			return nil, fmt.Errorf("API Append operation unsuccessful:\nDomain: %s\nHostname: %s\nReply code: %d\nDetails: %s",
				getDomain(zone), getHostname(zone, record.Name), reply.Code, reply.Detail)
		}
		appendedRecords = append(appendedRecords, record)
	}

	return appendedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing records or creating new ones.
// It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	log.Println("SetRecords", zone, records)

	domain := getDomain(zone)

	currentRecords, err := p.GetRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	var updateRecords []libdns.Record
	var appendRecords []libdns.Record

	for _, record := range records {
		if record.ID == "" {
			for i, currentRecord := range currentRecords {
				if currentRecord.Type == record.Type && getHostname(zone, currentRecord.Name) == getHostname(zone, record.Name) {
					currentRecords = append(currentRecords[:i], currentRecords[i+1:]...)
					record.ID = currentRecord.ID
					updateRecords = append(updateRecords, record)
					break
				}
				if i == len(currentRecords)-1 {
					appendRecords = append(appendRecords, record)
				}
			}
		} else {
			updateRecords = append(updateRecords, record)
		}

	}

	var updatedRecords []libdns.Record
	appendedRecords, err := p.AppendRecords(ctx, zone, appendRecords)
	if err != nil {
		return nil, err
	}
	copy(updatedRecords[:], appendedRecords[:])

	for _, record := range updateRecords {
		log.Println("updating record id " + record.ID)
		rrttl := ""
		if record.TTL != time.Duration(0) {
			rrttl = fmt.Sprintf("&rrttl=%d", int64(record.TTL/time.Second))
		}

		rrdistance := ""
		if record.Priority != 0 {
			rrdistance = fmt.Sprintf("&rrdistance=%d", record.Priority)
		}

		req_url := p.getApiHost() + "/dnsUpdateRecord?version=1&type=xml&key=" + p.APIToken + "&domain=" + domain +
			"&rrid=" + record.ID + "&rrhost=" + getHostname(zone, record.Name) + "&rrvalue=" + record.Value +
			rrdistance + rrttl
		req, err := http.NewRequest("GET", req_url, nil)
		if err != nil {
			return nil, err
		}

		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP error: Domain: %s; Records: %v, Status: %v; Body: %s",
				zone, currentRecords, resp.StatusCode, string(bodyBytes))
		}

		var reply struct {
			Code   int    `xml:"reply>code"`
			Detail string `xml:"reply>detail"`
		}

		err = xml.Unmarshal(bodyBytes, &reply)
		if err != nil {
			return nil, fmt.Errorf("didn't expect error: %s", err)
		}

		if reply.Code != 300 {
			return nil, fmt.Errorf("API Update operation failed:\nDomain: %s\nRecord: %s\nReply code: %d\nStatus: %s",
				getDomain(zone), getHostname(zone, record.Name), reply.Code, reply.Detail)
		}

		updatedRecords = append(updatedRecords, record)
	}

	return updatedRecords, nil
}

// DeleteRecords deletes the records from the zone. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	log.Println("DeleteRecords", zone, records)

	domain := getDomain(zone)

	currentRecords, err := p.GetRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	var deletedRecords []libdns.Record
	var deleteRecords []libdns.Record

	for _, record := range records {
		for i, currentRecord := range currentRecords {
			if currentRecord.Type == record.Type && getHostname(zone, currentRecord.Name) == getHostname(zone, record.Name) {
				currentRecords = append(currentRecords[:i], currentRecords[i+1:]...)
				deleteRecords = append(deleteRecords, currentRecord)
				break
			}
		}
	}

	for _, record := range deleteRecords {
		req_url := p.getApiHost() + "/dnsDeleteRecord?version=1&type=xml&key=" + p.APIToken + "&domain=" + domain + "&rrid=" + record.ID
		req, err := http.NewRequest("GET", req_url, nil)
		if err != nil {
			return nil, err
		}

		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP error: Domain: %s; Records: %v, Status: %v; Body: %s",
				zone, currentRecords, resp.StatusCode, string(bodyBytes))
		}

		var reply struct {
			Code   int    `xml:"reply>code"`
			Detail string `xml:"reply>detail"`
		}

		err = xml.Unmarshal(bodyBytes, &reply)
		if err != nil {
			return nil, fmt.Errorf("didn't expect error: %s", err)
		}

		if reply.Code != 300 {
			return nil, fmt.Errorf("API Delete operation unsuccessful:\nDomain: %s\nRecord: %s\nReply code: %d\nStatus: %s",
				getDomain(zone), getHostname(zone, record.Name), reply.Code, reply.Detail)
		}

		deletedRecords = append(deletedRecords, record)
	}

	return deletedRecords, nil
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
