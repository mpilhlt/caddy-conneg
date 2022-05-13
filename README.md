# Caddy Content Negotiation Plugin

[Content negotiation](https://en.wikipedia.org/wiki/Content_negotiation) is a mechanism of HTTP that allows client and server to agree on the best version of a resource to be delivered for the client's needs given the server's capabilities (see [RFC](https://datatracker.ietf.org/doc/html/rfc7231#section-5.3)). In short, when sending the request, the client can specify what *content type*, *language*, *character set* or *encoding* it prefers and the server responds with the best available version to fit the request.

This plugin to the [Caddy 2](https://caddyserver.com/) webserver that allows you to configure [named matchers](https://caddyserver.com/docs/caddyfile/matchers#named-matchers) for content negotiation parameters and/or store content negotiation results in [variables](https://caddyserver.com/docs/conventions#placeholders).

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

* `match_types` takes one or more (space-delimited) content types (a.k.a. mime types) that are available in this matcher. If the client requests a type (via HTTP's `Accept:` request header) compatible with one of those, the matcher returns true, if the request specifies types that cannot be satisfied by this list of offered types, the matcher returns false.
* `force_type_query_string` allows to specify a URL query parameter's key the value of which will override the HTTP `Accept:` header. It works in both ways, i.e. it can cause and prevent a match.
* `var_type` allows you to define a string that, prefixed with `conneg_` specifies a variable name that will store the result of content type negotiation, i.e. the best content type according to the types and weights specified by the client and what is on offer by the server. You can access this variable with `{vars.conneg_<name>` in other places pf your configuration.
* The same holds for languages (requested with the `Accept-Language` header), character sets (requested with the `Accept-Charset:` header), and encodings (which in reality are rather compression methods like `zip`, `deflate`, `compress` etc.; requested with the `Accept-Encoding` header).
* The requirements in the same named matcher are AND'ed together. If you want to OR, i.e. match alternatively, just configure multiple named matchers.

A [Caddyfile](./Caddyfile) with some combinations for testing is provided with this repository.

## Libraries

The plugin relies heavily on [elnormous/contenttype](https://github.com/elnormous/contenttype) and go's own [x/text/language](https://pkg.go.dev/golang.org/x/text/language) libraries. (For the intricacies of language negotiation, you may want to have a glance at the [blog post](https://go.dev/blog/matchlang) that accompanied the release of go's language library.)
Some other libraries are mentioned in the [`connegmatcher.go` file](./connegmatcher.go). I've come across most of them in [this go issue](https://github.com/golang/go/issues/19307).

## License

This software is licensed under the Apache License, Version 2.0.
