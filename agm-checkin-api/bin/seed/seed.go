package main

import (
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"johndpete316/agm-checkin-api/internal/db"
)

var firstNames = []string{
	"James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda",
	"William", "Barbara", "David", "Susan", "Richard", "Jessica", "Joseph", "Sarah",
	"Thomas", "Karen", "Charles", "Lisa", "Christopher", "Nancy", "Daniel", "Betty",
	"Matthew", "Margaret", "Anthony", "Sandra", "Mark", "Ashley", "Donald", "Emily",
	"Steven", "Donna", "Paul", "Michelle", "Andrew", "Carol", "Joshua", "Amanda",
	"Kenneth", "Melissa", "Kevin", "Deborah", "Brian", "Stephanie", "George", "Rebecca",
	"Timothy", "Sharon", "Ronald", "Laura", "Edward", "Cynthia", "Jason", "Kathleen",
	"Jeffrey", "Amy", "Ryan", "Angela", "Jacob", "Shirley", "Gary", "Anna",
	"Nicholas", "Brenda", "Eric", "Pamela", "Jonathan", "Emma", "Stephen", "Nicole",
	"Larry", "Helen", "Justin", "Samantha", "Scott", "Katherine", "Brandon", "Christine",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
	"Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson",
	"Thomas", "Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson",
	"White", "Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker",
	"Young", "Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
	"Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
	"Carter", "Roberts",
}

var divisions = []string{
	"Piano - Under 12",
	"Piano - 13-18",
	"Piano - Open",
	"Voice - Under 12",
	"Voice - 13-18",
	"Voice - Open",
	"Guitar - Under 12",
	"Guitar - 13-18",
	"Guitar - Open",
	"Violin - Under 12",
	"Violin - 13-18",
	"Violin - Open",
	"Cello - Under 12",
	"Cello - 13-18",
	"Cello - Open",
	"Flute - Under 12",
	"Flute - 13-18",
	"Flute - Open",
	"Trumpet - Under 12",
	"Trumpet - 13-18",
	"Trumpet - Open",
	"Percussion - Under 12",
	"Percussion - 13-18",
	"Percussion - Open",
}

// Spread check-ins across 3 event days
var eventDays = []time.Time{
	time.Date(2025, 6, 13, 0, 0, 0, 0, time.Local),
	time.Date(2025, 6, 14, 0, 0, 0, 0, time.Local),
	time.Date(2025, 6, 15, 0, 0, 0, 0, time.Local),
}

func randomCheckinTime(day time.Time, rng *rand.Rand) time.Time {
	minutes := rng.Intn(60 * 10) // within a 10-hour window
	return day.Add(8*time.Hour + time.Duration(minutes)*time.Minute)
}

func birthDateForDivision(division string, rng *rand.Rand) time.Time {
	baseYear := 2025
	var age int
	switch {
	case strings.Contains(division, "Under 12"):
		age = 7 + rng.Intn(5) // 7–11
	case strings.Contains(division, "13-18"):
		age = 13 + rng.Intn(6) // 13–18
	default: // Open
		age = 18 + rng.Intn(42) // 18–59
	}
	month := time.Month(1 + rng.Intn(12))
	day := 1 + rng.Intn(27)
	return time.Date(baseYear-age, month, day, 0, 0, 0, 0, time.UTC)
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	database := db.Connect(dsn)
	db.AutoMigrate(database)
	database.Where("1 = 1").Delete(&db.Competitor{})

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var competitors []db.Competitor
	seen := make(map[string]bool)

	for len(competitors) < 100 {
		first := firstNames[rng.Intn(len(firstNames))]
		last := lastNames[rng.Intn(len(lastNames))]
		name := first + " " + last
		if seen[name] {
			continue
		}
		seen[name] = true

		division := divisions[rng.Intn(len(divisions))]

		isCheckedIn := rng.Float32() < 0.4
		var checkInDateTime *time.Time
		if isCheckedIn {
			t := randomCheckinTime(eventDays[rng.Intn(len(eventDays))], rng)
			checkInDateTime = &t
		}

		competitors = append(competitors, db.Competitor{
			Name:               name,
			Division:           division,
			DateOfBirth:        birthDateForDivision(division, rng),
			RequiresValidation: rng.Float32() < 0.25,
			IsCheckedIn:        isCheckedIn,
			CheckInDateTime:    checkInDateTime,
		})
	}

	result := database.Create(&competitors)
	if result.Error != nil {
		log.Fatal("failed to seed competitors:", result.Error)
	}

	log.Printf("seeded %d competitors", len(competitors))
}
