package com.acme.model.vo.hello;

// Code generated by protoc-gen-bean. DO NOT EDIT.
// 2018-03-14 Wed 01:29:21 UTC+0800
//
//     hello.proto
//

public enum Greeting {

    Unknown(-1),
    NONE(0),
    MR(1),
    MRS(2),
    MISS(3);
    
    public int code;
    
    Greeting(int code) { this.code = code; }
    
    public static Greeting valueOf(final int code) {
        for (Greeting c : Greeting.values()) {
            if (code == c.code) return c;
        }
        return Unknown;
    }
}
