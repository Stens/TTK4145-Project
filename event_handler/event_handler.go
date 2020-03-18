package event_handler

import (
	"fmt"
	"strconv"
	"time"

	"../elevio"
	"../network/peers"
	"../order_distributor"
	"../sync/elevators"
	"../sync/nextfloor"
	"../sync/store"
)

var selfIP string

// RunElevator Her skjer det
func RunElevator(elevNumber int) {

	// First we start the server
	fmt.Println("Starting elevator server ...")
	selfIP = peers.GetRelativeTo(peers.Self, 0)
	connPort := strconv.Itoa(15657 + elevNumber)
	time.Sleep(time.Duration(1 * time.Second)) // To avoid crash due to not started sim
	elevio.Init("localhost:"+connPort, store.NumFloors)

	drv_buttons := make(chan elevio.ButtonEvent)
	drv_floors := make(chan int)
	drv_obstr := make(chan bool)
	drv_stop := make(chan bool)
	nextFloor := make(chan int)

	go elevio.PollButtons(drv_buttons) // Etasje og hvilken type knapp som blir trykket
	go elevio.PollFloorSensor(drv_floors)
	go elevio.PollObstructionSwitch(drv_obstr)
	go elevio.PollStopButton(drv_stop)
	go nextfloor.SubscribeToDestinationUpdates(nextFloor)

	// Initialize all elevators at the bottom when the program is first run.
	store.SetCurrentFloor(selfIP, store.NumFloors)

	goToFloor(0, drv_floors)

	for {
		select {
		case a := <-drv_buttons: // Just sets the button lamp, need to translate into calls
			// Setter på lyset
			light := DetermineLight(a.Floor, a.Button)
			elevio.SetButtonLamp(a.Button, a.Floor, light)

			// Håndtere callen
			if a.Button == elevio.BT_Cab {
				store.AddCabCall(selfIP, a.Floor)
			} else {
				elevDir := btnDirToElevDir(a.Button)
				mostSuitedIP := store.MostSuitedElevator(a.Floor, elevDir)

				// Create and send HallCall
				hc := elevators.HallCall_s{Floor: a.Floor, Direction: elevDir}
				order_distributor.SendHallCall(mostSuitedIP, hc)
			}

		case floor := <-nextFloor:

			goToFloor(floor, drv_floors)
			break

		}
	}
}

func goToFloor(destinationFloor int, drv_floors <-chan int) {

	direction := elevators.DirectionIdle
	currentFloor, _ := store.GetCurrentFloor(selfIP)
	if currentFloor < destinationFloor {
		direction = elevators.DirectionUp
	} else if currentFloor > destinationFloor {
		direction = elevators.DirectionDown
	}

	elevio.SetMotorDirection(direction)
	store.SetDirectionMoving(selfIP, direction)
	for {
		select {
		case floor := <-drv_floors: // Wait for elevator to reach floor
			elevio.SetFloorIndicator(floor)
			if floor == destinationFloor {
				arrivedAtFloor(floor)
				return
			}
			break
		case <-time.After(10 * time.Second):
			// fmt.Println("Didn't reach floor in time!")
			elevio.SetMotorDirection(elevators.DirectionIdle)
			//Do some shit
			return
			// break
		}
	}
}

func arrivedAtFloor(floor int) {
	store.SetCurrentFloor(selfIP, floor)
	elevio.SetMotorDirection(elevators.DirectionIdle) // Stop elevator and set lamps and stuff
	store.SetDirectionMoving(selfIP, elevators.DirectionIdle)
	elevio.SetFloorIndicator(floor)
	elevio.SetButtonLamp(elevio.BT_HallUp, floor, false)
	elevio.SetButtonLamp(elevio.BT_HallDown, floor, false)
	elevio.SetButtonLamp(elevio.BT_Cab, floor, false)
	elevio.SetDoorOpenLamp(true)
	time.Sleep(2 * time.Second)
	elevio.SetDoorOpenLamp(false)
}

func btnDirToElevDir(btn elevio.ButtonType) elevators.Direction_e {
	switch btn {
	case elevio.BT_HallDown:
		return elevators.DirectionDown
	case elevio.BT_HallUp:
		return elevators.DirectionUp
	default:
		return elevators.DirectionIdle
	}
}

func DetermineLight(floor int, button elevio.ButtonEvent) bool {
	localElevator, _ := store.GetElevator(selfIP)
	if floor == localElevator.GetCurrentFloor() && localElevator.GetDirectionMoving() == 0 {
		return false // If elevator is standing still and at floor, dont accept
	}
	return true
}
