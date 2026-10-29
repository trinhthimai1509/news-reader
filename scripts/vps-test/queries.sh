#!/bin/sh
# Times typical and worst-case reader queries (5 calls each, no other load).
# Prints: case, median and max in ms, HTTP status.
B=${1:-http://127.0.0.1:8083}
SIMSRC=$(curl -s "$B/api/sources" | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{const s=JSON.parse(d).filter(x=>x.name.startsWith("Sim "));console.log(s.length?s[s.length-1].id:"")})')
CUR=$(curl -s "$B/api/categories" | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>console.log(JSON.parse(d).cursor))')
t() { name=$1; url=$2
  for i in 1 2 3 4 5; do curl -s -o /dev/null -w "%{time_total} %{http_code}\n" "$B$url"; done |
  sort -n | awk -v n="$name" '{a[NR]=$1; c=$2} END {printf "%-40s median %7.1f ms  max %7.1f ms  HTTP %s\n", n, a[3]*1000, a[5]*1000, c}'; }
t "trang chủ (Tất cả, trang 1)" "/api/articles?page=1"
t "chuyên mục Thế giới" "/api/articles?category=the-gioi"
t "Chính trị (chỉ có feed giả lập)" "/api/articles?category=chinh-tri"
t "quốc gia GB" "/api/articles?country=GB"
t "quốc gia chưa xác định (không có)" "/api/articles?country=unknown"
[ -n "$SIMSRC" ] && t "một feed giả lập (nguồn $SIMSRC)" "/api/articles?source=$SIMSRC"
t "nguồn ít bài (Tuổi Trẻ Giáo dục, 28)" "/api/articles?source=28"
t "tìm 'việt' (có nhiều kết quả)" "/api/articles?q=vi%E1%BB%87t"
t "tìm từ không có (quét toàn bộ)" "/api/articles?q=zzqxk"
t "trang 200" "/api/articles?page=200"
t "danh sách chuyên mục + chấm đỏ (mốc cũ)" "/api/categories?seen=thoi-su:1,the-gioi:1,kinh-te:1,khoa-hoc-cong-nghe:1,chinh-tri:1,quan-su:1,doi-song:1,khac:1"
t "danh sách chuyên mục (mốc mới)" "/api/categories?seen=thoi-su:$CUR,khac:$CUR"
t "chi tiết bài" "/api/articles/$(curl -s "$B/api/articles" | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>console.log(JSON.parse(d).items[0].id))')"
t "nguồn (bộ lọc)" "/api/sources"
t "thống kê" "/api/stats"
