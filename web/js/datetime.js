"use strict";

(function($) {
    $.extend({
        myTime: {
            /**
             * @return <int> 
             */
            CurTime: function(){
                return Date.parse(new Date())/1000;
            },
            /**              
             * @param <string> 2014-01-01 20:20:20 
             * @return <int>               
             */
            DateToUnix: function(string) {
                var f = string.split(' ', 2);
                var d = (f[0] ? f[0] : '').split('-', 3);
                var t = (f[1] ? f[1] : '').split(':', 3);
                return (new Date(
                        parseInt(d[0], 10) || null,
                        (parseInt(d[1], 10) || 1) - 1,
                        parseInt(d[2], 10) || null,
                        parseInt(t[0], 10) || null,
                        parseInt(t[1], 10) || null,
                        parseInt(t[2], 10) || null
                        )).getTime() / 1000;
            },
            /**              
             * @param <int> unixTime  
             * @param <bool> isFull  
             * @param <int>  timeZone 
             */
            UnixToDate: function(unixTime, isFull, timeZone) {
                if (typeof (timeZone) == 'number')
                {
                    unixTime = parseInt(unixTime) + parseInt(timeZone) * 60 * 60;
                }
                var time = new Date(unixTime * 1000);
                var ymdhis = "";
                ymdhis += time.getUTCFullYear() + "-";
                if (time.getUTCMonth()+1 < 10) {
                    ymdhis += "0"
                }
                ymdhis += (time.getUTCMonth()+1) + "-";
                if (time.getUTCDate() < 10) {
                    ymdhis += "0"
                }
                ymdhis += time.getUTCDate();
                if (isFull === true)
                {
                    ymdhis += " "
                    if (time.getUTCHours() < 10) {
                        ymdhis += "0"
                    }
                    ymdhis += time.getUTCHours() + ":";
                    if (time.getUTCMinutes() < 10) {
                        ymdhis += "0"
                    }
                    ymdhis += time.getUTCMinutes() + ":";
                    if (time.getUTCSeconds() < 10) {
                        ymdhis += "0"
                    }
                    ymdhis += time.getUTCSeconds();
                }
                return ymdhis;
            }
        }
    });
})(jQuery);
