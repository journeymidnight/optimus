"use strict";

function getUrlParam(name) {
    var reg = new RegExp("(^|&)" + name + "=([^&]*)(&|$)"); 
    var r = window.location.search.substr(1).match(reg); 
    if (r != null) return unescape(r[2]); return null; 
}

function alert(type, strongMessage, message) {
    var div = '<div class="alert alert-TYPE fade in shadow">' +
            '<a href="#" class="close" data-dismiss="alert" aria-label="close">&times;</a>' +
            '<strong>STRONG </strong>' + 'MESSAGE' + '</div>';
    div = div.replace(/TYPE/g, type).replace(/MESSAGE/g, message).replace(/STRONG/g, strongMessage);
    $('#message').append(div);
}

function ajaxErrorHandler(data) {
    alert('danger', 'Error:', data.responseText);
    idle();
}

var result, displayResult
var ENTRIES_IN_PAGE=50
function displayOnePage(data, srcData) {
    var uSpeed = 0, dSpeed = 0;
    var table = [];
    var tpl = '<tr> <td>{{url}}</td> <td>{{size}}</td> <td>{{speed}}</td> <td>{{percentage}}</td> <td>{{status}}</td></tr>'

    for (var i = 0; i < data.length; i++) {
        var output = {}
        output['url'] = data[i]['url']
        output['size'] = data[i]['size']
        output['speed'] = data[i]['speed']
        output['percentage'] = data[i]['percentage']
        if (srcData[i]['url'] == data[i]['url']) {
            output['status'] = srcData[i]['status']
        } else {
            output['status'] = '----'
        }
        
        table.push(Mustache.render(tpl, output))
    }
    $('#resultTable > tbody').html(table);
}

function updateTable(data, page) {
    var finishedSize = 0
    
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
        $('#resultTable > tbody').html("");
    } else {
        $("#pagingDiv").removeClass('hide');
        var currPage = page
        var totalPages = Math.floor((data.length + ENTRIES_IN_PAGE - 1) / ENTRIES_IN_PAGE)
        
        var srcData = {}
        var urls = "["
        for (var i = start; i < end; i++) {
            var url = {}
            url['url'] = data[i]['url']
            urls += JSON.stringify(url)
            if (i + 1 != end) {
                urls += ','
            }
            srcData[i - start] = data[i]
        }
        urls += ']'
        var authHeader = getAuthHeader('POST', '/joburlsinfo', urls);
        $.ajax({
            url: '/joburlsinfo',
            type: 'POST',
            headers: authHeader,
            data: urls,
            success: function(data) {
                displayOnePage(data, srcData)
           },
            error: ajaxErrorHandler
        });
    }

    var value = currPage.toString() + ' / ' + totalPages.toString() 
    $("#jobPage").children("a:first").text(value)
}

function saveUrls(data, finished, pending, failed) {
    var entries = [];
    if(finished && data['success-files']) {
        data['success-files'].forEach(function(url) {
            var entry = {}
            entry['url'] = url
            entry['status'] = "Finished"
            entries.push(entry);
        })
    }
    if(pending && data['queued-files']) {
        data['queued-files'].forEach(function(url) {
            var entry = {}
            entry['url'] = url
            entry['status'] = "Pending"
            entries.push(entry);
        })
    }
    if(failed && data['failed-files']) {
        data['failed-files'].forEach(function(url) {
            var entry = {}
            entry['url'] = url
            entry['status'] = "Failed"
            entries.push(entry);
        })
    }
    return entries
}

function queryAJob(uuid) {
    var jobId = uuid;
    var urlsInfo
    var authHeader = getAuthHeader('GET', '/status');
    $.ajax({
        url: '/status?jobid=' + jobId,
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            result = data
            displayResult = saveUrls(result, true, true, true)
            updateTable(displayResult, 1);
            $('#resultTable').removeClass('hide');
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
    updateTable(displayResult, page);
    $("#jobPage").children("a:first").text(value)
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

function urlStatusChkBoxOnClick() {
    var finished = false, pending = false, failed = false
    if ($("#finishedChkBox").prop("checked")) {
        finished = true
    } 
    if ($("#pendingChkBox").prop("checked")) {
        pending = true
    }
    if ($("#failedChkbox").prop("checked")) {
        failed = true
    }
    displayResult = saveUrls(result, finished, pending, failed)
    updateTable(displayResult, 1);
}

function init() {
    $('#pagingUl').on('click','li', pagingClickEvent);

    var uuid = getUrlParam('uuid')
    if (uuid != null) {
        console.log(uuid)
        queryAJob(uuid)
    }

    refreshStaticsDiv();
    setInterval("refreshStaticsDiv();",10000);

    $('#finishedChkBox').click(urlStatusChkBoxOnClick);
    $('#pendingChkBox').click(urlStatusChkBoxOnClick);
    $('#failedChkbox').click(urlStatusChkBoxOnClick);

    //setTimeout('window.location.reload()',2000)
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