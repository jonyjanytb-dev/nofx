package market

import "testing"

func TestOnlyClosedKlines(t *testing.T){
	in:=[]Kline{{CloseTime:1000,Close:1},{CloseTime:2000,Close:2},{CloseTime:3000,Close:3}}
	out:=onlyClosedKlines(in,2500)
	if len(out)!=2||out[1].Close!=2{t.Fatalf("unexpected closed bars: %+v",out)}
}
