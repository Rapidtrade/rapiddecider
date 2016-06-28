# RapidDecider
This is the Decider program for RapidAPI workflows.  
It deals with scheduling activities for the workflows when orders, POD's Deliveries etc. come in via the modify3 method.  

The workflow will make the following decisions:  
1. handleWorkflowStart - will schedule a postorder activity and the task list of the supplierID  
2. handlePostorderComplete - will complete the workflow  

If the workflow either times out of fails, the decider will email the help desk.  

# Installing source and building  
    
    cd $GOPATH/github.com/Rapidtrade
    
