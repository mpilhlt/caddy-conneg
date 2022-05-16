# Caddy Content Negotiation Plugin

[Content negotiation](https://en.wikipedia.org/wiki/Content_negotiation) is a mechanism of HTTP that allows client and server to agree on the best version of a resource to be delivered for the client's needs given the server's capabilities (see [RFC](https://datatracker.ietf.org/doc/html/rfc7231#section-5.3)). In short, when sending the request, the client can specify what *content type*, *language*, *character set* or *encoding* it prefers and the server responds with the best available version to fit the request.

This plugin to the [Caddy 2](https://caddyserver.com/) webserver allows you to configure [named matchers](https://caddyserver.com/docs/caddyfile/matchers#named-matchers) for content negotiation parameters and/or store content negotiation results in [variables](https://caddyserver.com/docs/conventions#placeholders).

The plugin can be configured via Caddyfile:

## Syntax

```Caddyfile
@name {
    conneg {
        match_types <content-types...>
        force_type_query_string <name>
        var_type <name>

        match_languages <language codes...>
        force_language_query_string <name>
        var_language <name>

        match_charsets <character sets...>
        force_charset_query_string <name>
        var_charset <name>

        match_encoding <language codes...>
        force_encoding_query_string <name>
        var_encoding <name>
    }
}
```

* `match_types` takes one or more (space-separated) content types (a.k.a. mime types) that are available in this matcher. If the client requests a type (via HTTP's `Accept:` request header) compatible with one of those, the matcher returns true, if the request specifies types that cannot be satisfied by this list of offered types, the matcher returns false.
* `force_type_query_string` allows the client to specify a URL query parameter to override the HTTP `Accept:` header. (Say you want to download an `application/rdf+xml` file in the browser. Then the browser's default `Accept:` header will negotiate for a `text/html` version of the resource, but by specifying `?format=rdf`, you can "manually" request your desired content type.) It works in both ways, i.e. it can cause and prevent a match. In order not to require typing full content types on the URL, there is a [list of aliases](https://github.com/mpilhlt/caddy-conneg/blob/e3feae31ac8dc1a8066e60bd50e96e35c2ec9052/connegmatcher.go#L81) hardcoded that allows URLs like `...com/test?format=rdf` to be treated as equivalent to requesting `application/rdf+xml`. Suggestions for extending the list are welcome, please open an issue for that.
* `var_type` allows you to define a string that, prefixed with `conneg_`, specifies a variable name that will store the result of the content type negotiation, i.e. the best content type according to the types and weights specified by the client and what is on offer by the server. You can access this variable with `{vars.conneg_<name>}` in other places of your configuration.
* All of the above are repeated for *languages* (requested with the `Accept-Language:` header), *character sets* (requested with the `Accept-Charset:` header), and *encodings* (which in reality are rather compression methods like `zip`, `deflate`, `compress` etc., requested with the `Accept-Encoding:` header).
* Requirements in the same named matcher are AND'ed together. If you want to OR, i.e. match alternatively, just configure multiple named matchers.
* You must specify at least one of `match_types`, `match_languages`, `match_charsets`, and `match_encodings`. And when you specify one of the `var_*` parameters, the corresponding `match_` parameter must be defined as well.
* Wildcards like `*` and `*/*` should work. If they don't behave as you expect, please open an issue.

A [Caddyfile](./Caddyfile) with some combinations for testing is provided with this repository. You can test it with commands like these:

```shell
$ curl -H "Accept: application/tei+xml" -H "Accept-Language: fr-FR" https://localhost/test?format=rdf\&lang=de\&enc=br
RDF auf deutsch oder englisch, de preferred!
$ curl -H "Accept: application/tei+xml" -H "Accept-Language: fr-FR" https://localhost/test?format=rdf
RDF en français!
$ curl -H "Accept: application/rdf+xml" -H "Accept-Language: en" https://localhost/test?lang=de
RDF auf deutsch oder englisch, de preferred!
$ curl -H "Accept: application/rdf+xml" -H "Accept-Language: en" https://localhost/test
RDF auf deutsch oder englisch, English / English preferred!
$ curl -H "Accept: application/rdf+xml" -H "Accept-Language: en, de;q=0.8" https://localhost/test
RDF auf deutsch oder englisch, English / English preferred!
$ curl -H "Accept: application/rdf+xml" -H "Accept-Language: de-DE" https://localhost/test
RDF auf deutsch oder englisch, German / Deutsch preferred!
$ curl -H "Accept: application/rdf+xml" https://localhost/test
RDF!
$ curl -H "Accept: text/html" -H "Accept-Language: fr-FR" -H "Accept-Encoding: br" https://localhost/test?format=html\&lang=de
HTML, but brotli-compressed!
```

## Libraries

The plugin relies heavily on [elnormous/contenttype](https://github.com/elnormous/contenttype) and go's own [x/text/language](https://pkg.go.dev/golang.org/x/text/language) libraries. (For the intricacies of language negotiation, you may want to have a glance at the [blog post](https://go.dev/blog/matchlang) that accompanied the release of go's language library.) The charset and encoding negotiation mechanisms that I have developed for this plugin are somewhat simplistic, by contrast.

Some other content negotiation libraries that I have consulted are mentioned (but not used) in the [`connegmatcher.go` file](./connegmatcher.go). I've come across most of them in [this go issue](https://github.com/golang/go/issues/19307).

## License

This software is licensed under the Apache License, Version 2.0.
