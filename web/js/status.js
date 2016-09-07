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

var result
var ENTRIES_IN_PAGE=5
function updateTable(data, page) {
    var finishedSize = 0
    var uSpeed = 0, dSpeed = 0;
    var table = [];
    var tpl = '<tr> <td>{{url}}</td> <td>{{size}}</td> <td>{{speed}}</td> <td>{{percentage}}</td> <td>{{status}}</td></tr>'
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

        for (var i = 0; i < data.length; i++) {
            if (data[i]['size'] == -1) {
                data[i]['size'] = 'unknown'
            }
            if (data[i]['percentage'] == -1) {
                data[i]['percentage'] = 'unknown'
            }
            if (i >= start && i < end) {
                var output = Mustache.render(tpl, data[i])
                table.push(output)
            }
            
            if (data[i]['percentage'] == 100) {
                finishedSize += data[i]['size']
            } else if (data[i]['percentage'] > 50) {
                uSpeed += data[i]['speed']
            } else if (data[i]['percentage'] > 0) {
                dSpeed += data[i]['speed']
            }

            console.log(data[i]['size'])
            $('#resultTable > tbody').html(table);
        }
        
    }

    var value = currPage.toString() + ' / ' + totalPages.toString() 
    $("#jobPage").children("a:first").text(value)

    document.getElementById('uploadSpeed').innerHTML = uSpeed
    document.getElementById('downloadSpeed').innerHTML = dSpeed
    document.getElementById('finishedSize').innerHTML = finishedSize
}

function queryAJob(uuid) {
    var jobId = uuid;

    var authHeader = getAuthHeader('GET', '/jobdetail');
    $.ajax({
        url: '/jobdetail?jobid=' + jobId,
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            result = data
            updateTable(data, 1);
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
    updateTable(result, page);
    $("#jobPage").children("a:first").text(value)
}

function init() {
    $('#pagingUl').on('click','li', pagingClickEvent);

    var uuid = getUrlParam('uuid')
    if (uuid != null) {
        console.log(uuid)
        queryAJob(uuid)
    }

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