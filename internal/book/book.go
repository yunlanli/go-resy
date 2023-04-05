package book

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lgrees/resy-cli/internal/utils/date"
	"github.com/lgrees/resy-cli/internal/utils/http"
)

type BookingDetails struct {
	VenueId string
	// YYYY-MM-DD HH:MM:SS
	BookingDateTime string
	PartySize       string
	// YYYY-MM-DD
	ReservationDate string
	// HH:MM:SS - HH:MM:SS
	ReservationTimes []date.TimeRange
	ReservationTypes []string
}

type Slot struct {
	Date struct {
		Start string
	}

	Config struct {
		Type  string
		Token string
	}
}

type FindResponse struct {
	Results struct {
		Venues []struct {
			Slots []Slot
		}
	}
}

type DetailsResponse struct {
	BookToken struct {
		Value string `json:"value"`
	} `json:"book_token"`
	User struct {
		PaymentMethods []struct {
			Id int64 `json:"id"`
		} `json:"payment_methods"`
	} `json:"user"`
}

type BookingConfig struct {
	ConfigId  string `json:"config_id"`
	Day       string `json:"day"`
	PartySize int64  `json:"party_size"`
}

func ToBookCmd(bookingDetails *BookingDetails, dryRun bool) string {
	resTypes := make([]string, 0)

	for _, resType := range bookingDetails.ReservationTypes {
		resTypes = append(resTypes, fmt.Sprintf("'%s'", resType))
	}

	types := strings.Join(resTypes, ",")

	resTimes := make([]string, 0)
	for _, timeRange := range bookingDetails.ReservationTimes {
		resTimes = append(resTimes, timeRange.ToString())
	}
	times := strings.Join(resTimes, ",")
	resyExec, _ := os.Executable()

	return fmt.Sprintf("%s book --bookingDateTime='%s' --venueId=%s --partySize=%s --reservationDate=%s --reservationTimes='%s' --reservationTypes=%s --dryRun=%t --wait true", resyExec, bookingDetails.BookingDateTime, bookingDetails.VenueId, bookingDetails.PartySize, bookingDetails.ReservationDate, times, types, dryRun)
}

func Book(bookingDetails *BookingDetails, dryRun bool) error {
	slots, err := fetchSlots(bookingDetails)
	if err != nil {
		return err
	}

	matchingSlots := findMatches(bookingDetails, slots)
	if len(matchingSlots) == 0 {
		return errors.New("no matching slots")
	}
	if dryRun {
		fmt.Printf("matching slots: %v\n", matchingSlots)
		return nil
	}

	err = book(bookingDetails, matchingSlots)
	if err != nil {
		return err
	}
	return nil
}

func WaitThenBook(bookingDetails *BookingDetails, dryRun bool) error {
	bookTime, err := date.ParseDateTime(bookingDetails.BookingDateTime)
	if err != nil {
		return err
	}

	duration := time.Until(*bookTime)
	if duration.Minutes() > 5 {
		return fmt.Errorf("cannot wait more than %f minutes to book", duration.Minutes())
	}
	time.Sleep(duration)

	return Book(bookingDetails, dryRun)
}

func fetchSlots(bookingDetails *BookingDetails) ([]Slot, error) {
	params := make(map[string]string)
	params["party_size"] = bookingDetails.PartySize
	params["venue_id"] = bookingDetails.VenueId
	params["day"] = bookingDetails.ReservationDate
	// Seemingly deprecated but still required by the resy API
	params["lat"] = "0"
	params["long"] = "0"

	body, statusCode, err := http.Get("https://api.resy.com/4/find", &http.Req{QueryParams: params})
	if err != nil {
		return nil, err
	}
	if statusCode != 200 {
		return nil, fmt.Errorf("failed to fetch slots for date, status code: %d", statusCode)
	} else {
		fmt.Printf("fetchSlots succeeded.\n")
	}

	var obj FindResponse
	err = json.Unmarshal(body, &obj)
	if err != nil {
		return nil, err
	}

	return obj.Results.Venues[0].Slots, nil
}

func findMatches(bookingDetails *BookingDetails, slots []Slot) (matches []Slot) {
	for _, slot := range slots {
		if isSlotMatch(bookingDetails, slot) {
			matches = append(matches, slot)
		}
	}
	return
}

func book(bookingDetails *BookingDetails, matchingSlots []Slot) error {
	var err error
	for _, slot := range matchingSlots {
		fmt.Printf("booking %v", slot)
		err = bookSlot(bookingDetails, slot)
		if err == nil {
			fmt.Printf("...OK\n")
			return nil
		} else {
			fmt.Printf("...FAILED	%v\n", err)
		}
	}

	return fmt.Errorf("could not book any matching slots: %w", err)
}

func bookSlot(bookingDetails *BookingDetails, slot Slot) error {
	// Get booking token
	partySize, _ := strconv.Atoi(bookingDetails.PartySize)
	bookingConfig := BookingConfig{
		ConfigId:  slot.Config.Token,
		Day:       bookingDetails.ReservationDate,
		PartySize: int64(partySize),
	}
	body, err := json.Marshal(bookingConfig)
	if err != nil {
		return err
	}

	stringifyResponse := func(resp []byte) string {
		if resp == nil {
			return "[EMPTY RESPONSE]"
		}

		return string(resp)
	}

	responseBody, statusCode, err := http.PostJSON("https://api.resy.com/3/details", &http.Req{Body: body})
	if err != nil {
		return err
	}
	if statusCode >= 400 || responseBody == nil {
		return fmt.Errorf(
			"failed to get booking details, status code: %d, msg: %v",
			statusCode,
			stringifyResponse(responseBody))
	}

	var details DetailsResponse
	_ = json.Unmarshal(responseBody, &details)

	// Actually book with token
	token := fmt.Sprintf("book_token=%s", url.PathEscape(details.BookToken.Value))
	var paymentDetails string
	if details.User.PaymentMethods != nil {
		if len(details.User.PaymentMethods) != 0 {
			body, _ := json.Marshal(struct {
				Id int64 `json:"id"`
			}{Id: details.User.PaymentMethods[0].Id})
			paymentDetails = fmt.Sprintf("struct_payment_method=%s", url.PathEscape(string(body)))
		}
	} else {
		fmt.Println("[ Warn ] user account has no payment method.")
	}
	sourceId := url.PathEscape("source_id=resy.com-venue-details")

	var form string
	if paymentDetails != "" {
		form = strings.Join([]string{token, paymentDetails, sourceId}, "&")
	} else {
		form = token
	}
	responseBody, statusCode, err = http.PostForm(
		"https://api.resy.com/3/book",
		&http.Req{Body: []byte(form)},
		&map[string]string{
                        "host": "api.resy.com",
                        "origin": "https://widgets.resy.com",
			"referer": "https://wdigets.resy.com/",
                        "x-origin": "https://widgets.resy.com",
                        "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/111.0",
		})
	if err != nil {
		return err
	}
	if statusCode >= 400 {
		return fmt.Errorf(
			"failed to book reservation, status code: %d, msg: %v",
			statusCode,
			stringifyResponse(responseBody))
	}

	return nil
}

func isSlotMatch(bookingDetails *BookingDetails, slot Slot) bool {
	pieces := strings.Split(slot.Date.Start, " ")
	slotTime := pieces[1]
	slotType := strings.ToLower(slot.Config.Type)
	isTypeMatch := false
	if len(bookingDetails.ReservationTypes) == 0 {
		isTypeMatch = true
	}
	isTimeMatch := false

	for _, time := range bookingDetails.ReservationTimes {
		if slotTime >= time.Start && slotTime <= time.End {
			isTimeMatch = true
			break
		}
	}
	for _, resType := range bookingDetails.ReservationTypes {
		if resType == slotType {
			isTypeMatch = true
			break
		}
	}

	return isTimeMatch && isTypeMatch
}
