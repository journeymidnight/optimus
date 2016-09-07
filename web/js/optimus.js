"use strict";

function showConfigureModal() {
    var accessKey = window.localStorage.getItem('accessKey') || "";
    var secretKey = window.localStorage.getItem("secretKey") || "";
    $('#accessKey').val(accessKey);
    $('#secretKey').val(secretKey);
    $('#keyConfigureModal').modal('show');
}

function showJobModal() {
    if(!checkKeyExistence()) return;
    $('#newJobModal').modal('show')
}

function alert(type, strongMessage, message) {
    var div = '<div class="alert alert-TYPE fade in shadow">' +
            '<a href="#" class="close" data-dismiss="alert" aria-label="close">&times;</a>' +
            '<strong>STRONG </strong>' + 'MESSAGE' + '</div>';
    div = div.replace(/TYPE/g, type).replace(/MESSAGE/g, message).replace(/STRONG/g, strongMessage);
    $('#message').append(div);
}

function busy() {
    $('#searchBusy').removeClass('hide');
    $('#searchIdle').addClass('hide');
}

function showMain() {
    $('#singlequery').addClass('hide');
    $('#listquery').removeClass('hide');
}

function showListQuery() {
    $('#listquery').removeClass('hide');
    $('#jobIdInput').addClass('hide');
    $('#showJob').addClass('hide');
    console.log("showListQuery")
}

function idle() {
    $('#searchBusy').addClass('hide');
    $('#searchIdle').removeClass('hide');
}

function checkKeyExistence() {
    if (!window.localStorage.getItem('accessKey') ||
        !window.localStorage.getItem('secretKey')) {
        alert('info', '', 'Please configure your keys first');
        setTimeout(showConfigureModal, 300);
        return false;
    }
    return true;
}

function ajaxErrorHandler(data) {
    alert('danger', 'Error:', data.responseText);
    idle();
}

var result
var ENTRIES_IN_PAGE=50
function updateJobList(data, page) {
    var table = [];
    var tpl = '<tr> <td><a href="status.html?uuid={{jobid}}">{{jobid}}</a></td> <td>{{create-time}}</td> <td>{{complete-time}}</td> <td>{{satus}}</td> </tr>'
    var start = (page - 1) * ENTRIES_IN_PAGE
    var end = page * ENTRIES_IN_PAGE
    if (end > data.length) {
        end = data.length
    }
    if (data.length == 0) {
        $("#pagingDiv").addClass('hide');
        var currPage = 0
        var totalPages = 0
        var value = currPage.toString() + ' / ' + totalPages.toString() 
        $("#jobPage").children("a:first").text(value)
    } else {
        $("#pagingDiv").removeClass('hide');
        var currPage = page
        var totalPages = Math.floor((data.length + ENTRIES_IN_PAGE - 1) / ENTRIES_IN_PAGE)
        for (var i = start; i < end; i++) {
            var output = Mustache.render(tpl, data[i])
            table.push(output)
        }
        $('#jobList > tbody').html(table);
        var value = currPage.toString() + ' / ' + totalPages.toString()
        $("#jobPage").children("a:first").text(value)
    }
}

function convertTime(data) {
    var formatedResult = new Array()
    for (var i = 0; i < data.length; i++) {
        var entry = {}
        entry['jobid'] = data[i]['jobid']
        if (data[i]['create-time'] != 0) {
            entry['create-time'] = $.myTime.UnixToDate(data[i]['create-time'], true, 8)
        } else {
            entry['create-time'] = '---'
        }
        if (data[i]['complete-time'] != 0) {
            entry['complete-time'] = $.myTime.UnixToDate(data[i]['complete-time'], true, 8)
        } else {
            entry['complete-time'] = '---'
        }
        entry['satus'] = data[i]['satus']
        formatedResult[i] = entry
    }
    return formatedResult
}

function queryOneJob() {
    $('#queryOneJob').blur();
    if(!checkKeyExistence()) return;

    var jobId = $('#jobIdInput').val().trim();
    if(jobId === '') {
        alert('danger', '', 'No job ID specified');
        return;
    }
    busy();
    var authHeader = getAuthHeader('GET', '/joblist');
    $.ajax({
        url: '/joblist?jobid=' + jobId,
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            result = convertTime(data)
            updateJobList(result, 1);
            $('#jobList').removeClass('hide');
            idle();
        },
        error: ajaxErrorHandler
    })
}

function queryMoreJobs() {
    var STATUS_FINISHED  = 1
    var STATUS_PENDING   = 2
    var STATUS_FAILED    = 4
    var STATUS_SCHEDULED = 8

    var url = "/joblist"
    var para = ""

    var stime = 0, etime = 0;
    if ($("#startTime").val() != "") {
        stime = $.myTime.DateToUnix($("#startTime").val())
    }
    if ($("#endTime").val() != "") {
        etime = $.myTime.DateToUnix($("#endTime").val())
    }
    if (stime != 0 && etime != 0 && stime >= etime)  {
        alert('danger', '', 'start time more than end time');
        return;
    }
    if (stime != 0) {
        para += "&stime=" + stime.toString()
    }
    if (etime != 0) {
        para += "&etime=" + etime.toString()
    }
    var status = 0
    if ($("#finishedChkBox").prop("checked")) {
        status += STATUS_FINISHED
    }
    if ($("#pendingChkBox").prop("checked")) {
        status += STATUS_PENDING
    }
    if ($("#failedChkbox").prop("checked")) {
        status += STATUS_FAILED
    }
    if (status != 0) {
        para += "&status=" + status.toString()
    }
    if (para.length != 0) {
        para = '?' + para.substr(1)
    }
    console.log(para)
    url += para
    var authHeader = getAuthHeader('GET', '/joblist');
    $.ajax({
        url: url,
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            result = convertTime(data)
            updateJobList(result, 1);
            $('#jobList').removeClass('hide');
            idle();
        },
        error: ajaxErrorHandler
    })
}

function verifyUrls(urls) {
    for(var i = 0; i < urls.length; i++) {
        try {
            new URL(urls[i]);
        } catch (e) {
            alert('danger', 'Invalid URL:', urls[i]);
            return false
        }
    }
    return true;
}

function submitJob() {
    if(!checkKeyExistence()) return;

    var urlValue = $('#urls').val();
    if(urlValue.trim() === "") {
        alert('danger', '', "No URLs specified");
        return;
    }
    var urls = urlValue.split('\n').map(function(url) {
        return url.trim()
    });
    if(!verifyUrls(urls)) {return;}
    var type = $('#type').val();
    var data;
    switch (type) {
        case "s3s":
        case "cn-north-1":
            var bucket = $('#bucket').val().trim();
            if(bucket === "") {
                alert('danger', '', 'No bucket specified');
                return;
            }
            data = {
                'origin-files': urls,
                'target-type': type,
                'target-bucket': bucket,
                'target-acl': $('#acl').val()
            };
            break;
        case 'Vaas':
            data = {
                'origin-files': urls,
                'target-type': type
            };
            break;
    }
    var dataString = JSON.stringify(data);
    var authHeader = getAuthHeader('PUT', '/transferjob', dataString);
    $.ajax({
        url: '/transferjob',
        type: 'PUT',
        headers: authHeader,
        data: dataString,
        success: function(data) {
            alert('success', 'Job ID:', data.jobid);
        },
        error: ajaxErrorHandler
    });
    $('#newJobModal').modal('hide');
}

function saveKeys() {
    var accessKey = $('#accessKey').val().trim();
    var secretKey = $('#secretKey').val().trim();
    window.localStorage.setItem('accessKey', accessKey);
    window.localStorage.setItem('secretKey', secretKey);
    $('#keyConfigureModal').modal('hide');
}

function refreshStaticsDiv(){
    var authHeader = getAuthHeader('GET', '/finishedsize');
    $.ajax({
        url: '/finishedsize',
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            document.getElementById('finishedSize').innerHTML = data['finished-size']
        },
        error: ajaxErrorHandler
    })
    var authHeader = getAuthHeader('GET', '/currentspeed');
    $.ajax({
        url: '/currentspeed',
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            document.getElementById('uploadSpeed').innerHTML = data['upload-speed']
            document.getElementById('downloadSpeed').innerHTML = data['download-speed']
        },
        error: ajaxErrorHandler
    })
}

function pagingClickEvent() {
    var page = 1
    var value = ''
    if ($(this).attr("id") == "prev") {
        var subStrs = $("#jobPage").children("a:first").text().split('/')
        var currPage = parseInt(subStrs[0])
        var totalPages = parseInt(subStrs[1])
        if (currPage > 1) {
            currPage -= 1
        }
        page = currPage
        value = currPage.toString() + ' / ' + totalPages.toString() 
    } else if ($(this).attr("id") == "next") {
        var subStrs = $("#jobPage").children("a:first").text().split('/')
        var currPage = parseInt(subStrs[0])
        var totalPages = parseInt(subStrs[1])
        if (currPage < totalPages) {
            currPage += 1
        }
        page = currPage
        value = currPage.toString() + ' / ' + totalPages.toString() 
    } else {
        return
    }
    updateJobList(result, page);
    $("#jobPage").children("a:first").text(value)
}

function init() {
    $('#newJob').click(showJobModal);
    $('#configureKeys').click(showConfigureModal);
    $('#jobIdInput').keypress(function(event) {
        var keycode = (event.keyCode ? event.keyCode : event.which);
        if(keycode == '13') { // Enter key
            queryOneJob();
        }
    });
    $('#queryOneJob').click(queryOneJob);
    $('#queryMoreJobs').click(queryMoreJobs);
    $('#jobSubmit').click(submitJob);
    $('#saveKeys').click(saveKeys);

    $(".form_datetime").datetimepicker({
        //language:  'fr',
        weekStart: 1,
        todayBtn:  1,
        autoclose: 1,
        todayHighlight: 1,
        startView: 2,
        forceParse: 0,
        showMeridian: 1
    });

    refreshStaticsDiv();
    setInterval("refreshStaticsDiv();",10000);

    $('#pagingUl').on('click','li', pagingClickEvent);

    var type = $('#type');
    var acl = $('#acl');
    var bucket = $('#bucket');
    type.change(function() {
        switch (type.val()) {
            case 's3s':
                acl.prop('disabled', false);
                bucket.prop('disabled', false);
                break;
            case 'Vaas':
                acl.prop('disabled', true);
                bucket.prop('disabled', true);
                break;
        }
    })
}

function getAuthHeader(method, urlPath, dataString) {
    var accessKey = window.localStorage.getItem('accessKey') || "";
    var secretKey = window.localStorage.getItem("secretKey") || "";
    var now = new Date().toUTCString();
    var message = method + '\n' + now + '\n' + CryptoJS.MD5(dataString).toString() + '\n' + urlPath;
    var hmac = CryptoJS.HmacSHA1(message, secretKey);
    var base64 = CryptoJS.enc.Base64.stringify(hmac);
    return {
        'x-date': now,
        'Authorization': accessKey + ':' + base64
    }
}

$(init);