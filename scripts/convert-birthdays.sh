awk -F',' 'BEGIN{OFS=","} NR==1{print; next} {
      n=split($7, d, "/")
      if(n==3) $7=sprintf("%04d-%02d-%02d", d[3], d[1], d[2])
      print
  }' temp.csv > temp_2.csv