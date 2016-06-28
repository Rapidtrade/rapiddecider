package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/CaboodleData/gotools/amazon"
	"github.com/CaboodleData/gotools/file"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/swf"
)

type decision struct {
	svc        *swf.SWF
	tt         string // task token associated with this decision
	runid      string //Workflows runid
	workflowid string //
}

type result struct {
	SupplierID string
	File       string
}

//Globals variables
var (
	Info        *log.Logger
	Error       *log.Logger
	swfDomain   = "Orders"
	stdout      bool
	helpdesk    = "shaun@rapidtrade.biz"
	swfTasklist = "OrderDecider"
	swfIdentity = "RapidDecider"
)

func main() {

	flag.BoolVar(&stdout, "stdout", false, "Log to stdout")
	flag.Parse()

	// initialise logs
	Info, Error = file.InitLogs(stdout, "/var/rapiddecider", "rapiddecider")
	Info.Println("Starting rapiddecider =================>")

	// start workflow
	swfsvc := swf.New(session.New(), &aws.Config{Region: aws.String("us-east-1")})
	params := &swf.PollForDecisionTaskInput{
		Domain: aws.String(swfDomain), //
		TaskList: &swf.TaskList{ //
			Name: aws.String(swfTasklist), //
		},
		Identity:        aws.String(swfIdentity),
		MaximumPageSize: aws.Int64(100),
		ReverseOrder:    aws.Bool(true),
	}

	// loop forever while polling for work
	for {
		resp, err := swfsvc.PollForDecisionTask(params)
		if err != nil {
			amazon.SESSendEmail("support@rapidtrade.biz", helpdesk, swfIdentity+" unable to pole", err.Error())
			Error.Printf("error: unable to poll for decision: %v\n", err)
			panic("Broken, check logs")
		}

		// if we do not receive a task token then 60 second time out occured so try again
		if resp.TaskToken != nil {
			if *resp.TaskToken != "" {
				d := &decision{
					svc:        swfsvc,
					tt:         *resp.TaskToken,
					runid:      *resp.WorkflowExecution.RunId,
					workflowid: *resp.WorkflowExecution.WorkflowId,
				}
				// make each decision in a goroutine which means that multiple decisions can be made
				go d.makeDecision(resp.Events, resp.WorkflowExecution.RunId)
			}
		} else {
			Info.Printf("debug - no decisions required\n")
		}
	}
}

func (d *decision) makeDecision(events []*swf.HistoryEvent, ID *string) {
	Info.Print("###############\n")
	Info.Print(ID)
	Info.Print("\n##############\n")

	var handled bool
	var err error

	// loop backwards through time and make decisions
	for k, event := range events {
		switch *event.EventType {
		case "WorkflowExecutionStarted":
			d.handleWorkflowStart(event)
			handled = true

		case "ActivityTaskCompleted":
			_ = "breakpoint"
			lastActivity := d.getLastScheduledActivity(events)
			switch string(lastActivity) {
			case "postorder":
				d.completeWorkflow("")
				handled = true
			}

		case "ActivityTaskTimedOut":
			d.handleTimeout()
			handled = true

		case "ActivityTaskFailed":
			Info.Println("Cancelling workflow")
			d.emailError("ActivityTaskFailed")
			d.failWorkflow(*event.ActivityTaskFailedEventAttributes.Reason, nil)
			handled = true

		case "ActivityTaskCanceled":
			d.failWorkflow("Workflow cancelled after activity cancelled", nil)
			handled = true

		case "WorkflowExecutionCancelRequested":
			d.failWorkflow("Workflow cancelled by request", nil)
			handled = true

		case "TimerFired":
			err = d.handleTimerFired(k, events)
			handled = true

		default:
			Info.Printf("Unhandled: %s\n", *event.EventType)
		}
		if handled == true {
			break // decision has been made so stop scanning the events
		}
	}

	if err != nil {
		Info.Printf("Error making decision. workflow failed: %v\n", err)
		// we are not able to process the workflow so fail it
		err2 := d.failWorkflow("", err)
		if err2 != nil {
			Info.Printf("error while failing workflow: %v\n", err2)
		}
	}

	if handled == false {
		Info.Printf("debug dump of received event for taskToken: %s\n", d.tt)
		Info.Println(events)
		Info.Printf("xxxx debug unhandled decision\n")
	}
	Info.Print("#################### completed handling Decision ####################\n")
	// exit goroutine
}

// ============================== generic functions =========================================
// getLastScheduledActivity loops through the workflow events in reverse order to pick up the details of the name of the last scheduled activity
func (d *decision) getLastScheduledActivity(events []*swf.HistoryEvent) string {
	for _, event := range events {
		if *event.EventType == "ActivityTaskScheduled" {
			return *event.ActivityTaskScheduledEventAttributes.ActivityType.Name
		}
	}
	return ""
}

func (d *decision) handleTimerFired(k int, es []*swf.HistoryEvent) error {
	return nil
}

func (d *decision) setTimer(sec, data, id string) error {
	Info.Printf("debug start set timer to wait: %s seconds\n", sec)

	params := &swf.RespondDecisionTaskCompletedInput{
		TaskToken: aws.String(d.tt),
		Decisions: []*swf.Decision{
			{
				DecisionType: aws.String("StartTimer"),
				StartTimerDecisionAttributes: &swf.StartTimerDecisionAttributes{
					StartToFireTimeout: aws.String(sec),
					TimerId:            aws.String(id),
					Control:            aws.String(data),
				},
			},
		},
		ExecutionContext: aws.String("ssec2-amicreate"),
	}
	_, err := d.svc.RespondDecisionTaskCompleted(params)
	return err
}

// handleTimeout will send an email if the first timeout, then set marker so next time we dont email
func (d *decision) handleTimeout() error {
	to, _ := d.emailError("timeout")
	params := &swf.RespondDecisionTaskCompletedInput{
		TaskToken: aws.String(d.tt),
		Decisions: []*swf.Decision{
			{
				DecisionType: aws.String("RecordMarker"),
				RecordMarkerDecisionAttributes: &swf.RecordMarkerDecisionAttributes{
					MarkerName: aws.String("HelpdeskNotified"),
					Details:    aws.String(to),
				},
			},
		},
		ExecutionContext: aws.String("Data"),
	}
	_, err := d.svc.RespondDecisionTaskCompleted(params)
	return err // which may be nil
}

func (d *decision) emailError(reason string) (string, error) {
	runid := strings.Replace(d.runid, "=", "!=", 1)
	msg := "https://console.aws.amazon.com/swf/home?region=us-east-1#execution_events:domain=" + swfDomain + ";workflowId=" + d.workflowid + ";runId=" + runid
	err := amazon.SESSendEmail("support@rapidtrade.biz", helpdesk, "Workflow "+reason+" Occured", msg)
	if err != nil {
		return "", err
	}
	Info.Printf("Error emailed to %v", helpdesk)
	return helpdesk, nil
}

// completeWorkflow will complete workflow
func (d *decision) completeWorkflow(result string) error {
	params := &swf.RespondDecisionTaskCompletedInput{
		TaskToken: aws.String(d.tt),
		Decisions: []*swf.Decision{
			{
				DecisionType: aws.String("CompleteWorkflowExecution"),
				CompleteWorkflowExecutionDecisionAttributes: &swf.CompleteWorkflowExecutionDecisionAttributes{
					Result: aws.String(result),
				},
			},
		},
		ExecutionContext: aws.String("Data"),
	}
	_, err := d.svc.RespondDecisionTaskCompleted(params)
	return err // which may be nil
}

// completeWorkflow will complete workflow
func (d *decision) failWorkflow(details string, err error) error {
	errorD := ""
	if err != nil {
		errorD = fmt.Sprintf("%v", err)
	}
	params := &swf.RespondDecisionTaskCompletedInput{
		TaskToken: aws.String(d.tt),
		Decisions: []*swf.Decision{
			{
				DecisionType: aws.String("FailWorkflowExecution"),
				FailWorkflowExecutionDecisionAttributes: &swf.FailWorkflowExecutionDecisionAttributes{
					Details: aws.String(details),
					Reason:  aws.String(errorD),
				},
			},
		},
		ExecutionContext: aws.String("Data"),
	}
	_, err = d.svc.RespondDecisionTaskCompleted(params)
	return err // which may be nil
}

// scheduleNextaActivity will start the next activity
func (d *decision) scheduleNextaActivity(tt string, name string, version string, input string, stcTimeout string, tasklist string, context string) error {
	id := name + time.Now().Format("200601021504")
	Info.Printf("Scheduling eventType: %s\n", name)
	params := &swf.RespondDecisionTaskCompletedInput{
		TaskToken: aws.String(tt),
		Decisions: []*swf.Decision{
			{
				DecisionType: aws.String("ScheduleActivityTask"), //
				ScheduleActivityTaskDecisionAttributes: &swf.ScheduleActivityTaskDecisionAttributes{
					ActivityId: aws.String(id),
					ActivityType: &swf.ActivityType{
						Name:    aws.String(name),
						Version: aws.String(version),
					},
					Input:               aws.String(input),
					StartToCloseTimeout: aws.String(stcTimeout),
					TaskList: &swf.TaskList{
						Name: aws.String(tasklist),
					},
				},
			},
		},
		ExecutionContext: aws.String(context),
	}
	_, err := d.svc.RespondDecisionTaskCompleted(params)
	return err
}

func (d *decision) getJSON(input string) map[string]interface{} {
	var data interface{}
	json.Unmarshal([]byte(input), &data)
	m := data.(map[string]interface{})
	return m
}

//======================================= handle routines ==================================================
// handleLoadBigQueryComplete gets result JSON and starts loadcompleted using the supplierid as the tasklist
func (d *decision) handlePostorderComplete(jsonstr string) error {
	var rslt result
	err := json.Unmarshal([]byte(jsonstr), &rslt)
	if err != nil {
		return err
	}
	err = d.scheduleNextaActivity(d.tt, "loadcompleted", "2", rslt.File, "10000", rslt.SupplierID, "")
	return err
}

func (d *decision) handleWorkflowStart(event *swf.HistoryEvent) error {
	wfInput := *event.WorkflowExecutionStartedEventAttributes.Input
	json := d.getJSON(wfInput)
	supplierid, _ := json["SupplierID"].(string)
	err := d.scheduleNextaActivity(d.tt, "postorder", "1", wfInput, "10000", supplierid, "")
	return err
}
