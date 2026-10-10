#!/bin/bash
# Autoscale the Next.js frontend PM2 cluster between MIN and MAX by avg CPU per instance.
set -euo pipefail
export PATH=/usr/bin:/usr/local/bin:$PATH
APP=filmorauz-frontend
MIN=2; MAX=4
UP=70    # avg cpu% per instance -> scale up
DOWN=20  # avg cpu% per instance -> scale down
stat=$(pm2 jlist 2>/dev/null | python3 -c "
import sys,json
try: data=json.load(sys.stdin)
except Exception: print('0 0'); sys.exit()
ps=[p for p in data if p.get('name')=='$APP' and p['pm2_env'].get('status')=='online']
n=len(ps)
avg=(sum(p.get('monit',{}).get('cpu',0) for p in ps)/n) if n else 0
print(n, round(avg))
")
COUNT=$(echo "$stat" | awk '{print $1}'); AVG=$(echo "$stat" | awk '{print $2}')
[ "${COUNT:-0}" -lt 1 ] && exit 0
NEW=$COUNT
if [ "$AVG" -gt "$UP" ] && [ "$COUNT" -lt "$MAX" ]; then NEW=$((COUNT+1)); fi
if [ "$AVG" -lt "$DOWN" ] && [ "$COUNT" -gt "$MIN" ]; then NEW=$((COUNT-1)); fi
if [ "$NEW" != "$COUNT" ]; then
  pm2 scale "$APP" "$NEW" >/dev/null 2>&1 || exit 0
  logger -t fe-autoscale "scaled $APP $COUNT -> $NEW (avg cpu ${AVG}%)"
fi
