package com.acme.model.vo.common;

// Code generated by protoc-gen-bean. DO NOT EDIT.
// 2018-03-13 Tue 19:54:26 UTC+0800
//
//     common.proto
//

public enum Cmd {

    Unknown(0),
    kHandshakeReq(1),
    kHandshakeRsp(2);
    
    public int code;
    
    Cmd(int code) { this.code = code; }
    
    public static Cmd valueOf(final int code) {
        for (Cmd c : Cmd.values()) {
            if (code == c.code) return c;
        }
        return Unknown;
    }
}
