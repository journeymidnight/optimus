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

function updateTable(data) {
    var tr = '<tr>' + '<td>URL</td>' + '<td>STATUS</td>' + '</tr>';
    var table = [];
    if(data['success-files']) {
        data['success-files'].forEach(function(url) {
            table.push(tr.replace(/URL/g, url).replace(/STATUS/g, 'Finished'));
        })
    }
    if(data['queued-files']) {
        data['queued-files'].forEach(function(url) {
            table.push(tr.replace(/URL/g, url).replace(/STATUS/g, 'Pending'));
        })
    }
    if(data['failed-files']) {
        data['failed-files'].forEach(function(url) {
            table.push(tr.replace(/URL/g, url).replace(/STATUS/g, 'Failed'));
        })
    }
    $('#resultTable > tbody').html(table);
}

function queryJob() {
    $('#showJob').blur();
    if(!checkKeyExistence()) return;

    var jobId = $('#jobIdInput').val().trim();
    if(jobId === '') {
        alert('danger', '', 'No job ID specified');
        return;
    }
    busy();
    var authHeader = getAuthHeader('GET', '/status');
    $.ajax({
        url: '/status?jobid=' + jobId,
        type: 'GET',
        headers: authHeader,
        success: function(data) {
            updateTable(data);
            $('#resultTable').removeClass('hide');
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
    var authHeader = getAuthHeader('PUT', '/transferjob');
    $.ajax({
        url: '/transferjob',
        type: 'PUT',
        headers: authHeader,
        data: JSON.stringify(data),
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

function init() {
    $('#newJob').click(showJobModal);
    $('#configureKeys').click(showConfigureModal);
    $('#jobIdInput').keypress(function(event) {
        var keycode = (event.keyCode ? event.keyCode : event.which);
        if(keycode == '13') { // Enter key
            queryJob();
        }
    });
    $('#showJob').click(queryJob);
    $('#jobSubmit').click(submitJob);
    $('#saveKeys').click(saveKeys);

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

function getAuthHeader(method, urlPath) {
    var accessKey = window.localStorage.getItem('accessKey') || "";
    var secretKey = window.localStorage.getItem("secretKey") || "";
    var now = new Date().toUTCString();
    var message = method + '\n' + now + '\n' + location.host + '\n' + urlPath;
    var hmac = CryptoJS.HmacSHA1(message, secretKey);
    var base64 = CryptoJS.enc.Base64.stringify(hmac);
    return {
        'x-date': now,
        'Authorization': accessKey + ':' + base64
    }
}

$(init);