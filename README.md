# RapidDecider
This is the Decider program for RapidAPI workflows.  
It deals with scheduling activities for the workflows when orders, POD's Deliveries etc. come in via the modify3 method.  

The workflow will make the following decisions:  
1. handleWorkflowStart - will schedule a postorder activity and the task list of the supplierID  
2. handlePostorderComplete - will complete the workflow  

If the workflow either times out of fails, the decider will email the help desk.  

# Installing source and building  

First download the source code and build  
    
    cd $GOPATH/github.com/Rapidtrade  
    git clone https://github.com/Rapidtrade/rapiddecider.git  
    cd rapiddecider  
    go get -v  
    go get github.com/jmespath/go-jmespath  
    go get github.com/go-ini/ini  
    go build rapiddecider.go  
    
Now get folders ready  

    sudo mkdir /opt/rapiddecider  
    sudo chmod 777 -R /opt/rapiddecider
    sudo mkdir /var/rapiddecider  
    sudo chmod 777 -R /var/rapiddecider
    sudo cp rapiddecider /opt/rapiddecider
    
Now lets check that it works, after running, you should see:  

    cd /opt/rapiddecider
    rapiddecider -stdout
    INFO: 2016/06/28 14:45:37 rapiddecider.go:48: Starting rapiddecider =================>

# Setup SystemD  

Setup your SystemD service called rapiddecider.service.  
You can follow here: http://rapidtrade.screenstepslive.com/s/standards/m/16189/l/559609-setup-a-systemd-service
    
    
