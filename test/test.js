import http from 'k6/http';
import { check } from 'k6';
import { Trend } from 'k6/metrics';

let responseTimeTrend = new Trend('response_time');

const url = 'http://IP:PORT/send'; 

export let options = {
  stages: [
    { duration: '10s', target: 10 },  
    { duration: '5s', target: 0 },   
  ],
};

export default function () {
  let userUSN = __ITER + 1;  

  const payload = JSON.stringify({
    user: `testUser${__VU}`,  
    USN: userUSN,             
    group: 'RaftMsgStream',
    message: `Hello from k6 test for user ${__VU} - USN: ${userUSN}`, 
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const res = http.post(url, payload, params);

  check(res, {
    'is status 200': (r) => r.status === 200,
  });

  responseTimeTrend.add(res.timings.duration);
}
