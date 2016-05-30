// Generated automatically: do not edit manually

package generator

import (
    "time"
)

type Uider interface {
    GetUid() string  
}

type Envelope struct {
    Uuid           string    `json:"uuid"`
    SequenceNumber uint64    `json:"sequenceNumber"`
    Timestamp      time.Time `json:"timestamp"`
    AggregateName  string    `json:"aggregateName"`
    AggregateUid   string    `json:"aggregateUid"`
    EventTypeName  string    `json:"eventTypeName"`
    EventData      string    `json:"eventData"`
}