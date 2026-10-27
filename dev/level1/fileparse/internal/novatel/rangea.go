package novatel

import (
	"errors"
	"fmt"

	novatelascii "gitlab.com/earthscope/gnsstools/codecs/novatel/novatel_ascii"
	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"
)

// ErrBadEpochTime marks a RANGEA whose epoch time fails
// gnssiceberg.ValidateEpoch: a decode error (the ASCII CRC is not checked),
// not data.
var ErrBadEpochTime = errors.New("bad epoch time")

// RANGEAEpoch decodes a RANGEA log into an observation epoch. An epoch with
// no satellites is an error, and one whose time fails
// gnssiceberg.ValidateEpoch an ErrBadEpochTime error.
func RANGEAEpoch(m novatelascii.LongMessage) (observation.Epoch, error) {
	rangea, err := novatelascii.DeserializeRANGEA(m.Data)
	if err != nil {
		return observation.Epoch{}, err
	}
	epoch, err := rangea.SerializeGNSSEpoch(m.Time())
	if err != nil {
		return observation.Epoch{}, err
	}
	if len(epoch.Satellites) == 0 {
		return observation.Epoch{}, fmt.Errorf("RANGEA with no satellites")
	}
	if err := gnssiceberg.ValidateEpoch(epoch.Time); err != nil {
		return observation.Epoch{}, fmt.Errorf("%w: %v", ErrBadEpochTime, err)
	}
	return epoch, nil
}
